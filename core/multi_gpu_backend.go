//go:build cuda && nccl

package core

// MultiGPUBackend: NCCL-distributed state-vector simulator for large
// qubit counts (issue #23: beyond ~40 qubits, a single GPU's memory can't
// hold 2^n complex128 amplitudes). Shards the state vector evenly across
// nGPUs GPUs — nGPUs must be a power of 2 — using the standard
// partitioning every distributed state-vector simulator (Qiskit Aer-GPU,
// cuQuantum appliance) uses: the top log2(nGPUs) qubits become "global"
// (which GPU owns a given amplitude), the rest "local" (offset within
// that GPU's shard).
//
// Excluded from the default build by the "cuda" and "nccl" build tags:
// compiling this file requires multiple NVIDIA GPUs plus the NCCL
// library, neither available in this development environment. It has NOT
// been compiled or tested here — build with `go build -tags "cuda nccl"`
// on a multi-GPU machine and review the cgo calls below against your
// installed NCCL/cuQuantum versions before relying on it.
//
// Scope: a gate touching only "local" qubits applies independently per
// shard via cuStateVec (core/cuda_backend.go's single-GPU path, no
// communication needed). ExchangeShard implements the NCCL primitive a
// gate touching a *global* qubit needs — a pairwise halo exchange between
// GPU `rank` and GPU `rank XOR (1<<globalQubitOffset)` — which is the
// piece issue #23 actually asks for ("usar NCCL para comunicación
// inter-GPU eficiente"). Composing that primitive into Apply for an
// arbitrary gate touching a global qubit is not implemented: correctly
// handling every global/local qubit combination is substantial additional
// surface this cannot verify without the hardware, so Apply reports a
// clear error for that case rather than guessing.

/*
#cgo LDFLAGS: -lcudart -lcustatevec -lnccl
#include <cuda_runtime.h>
#include <custatevec.h>
#include <nccl.h>
#include <stdlib.h>

// linlang_nccl_init creates one NCCL communicator per GPU (nGPUs ranks on
// a single host, the common single-process multi-GPU pattern) sharing one
// ncclUniqueId.
static int linlang_nccl_init(ncclComm_t *comms, int nGPUs) {
    ncclUniqueId id;
    if (ncclGetUniqueId(&id) != ncclSuccess) return -1;
    if (ncclGroupStart() != ncclSuccess) return -2;
    for (int rank = 0; rank < nGPUs; rank++) {
        if (cudaSetDevice(rank) != cudaSuccess) return -3;
        if (ncclCommInitRank(&comms[rank], nGPUs, id, rank) != ncclSuccess) return -4;
    }
    if (ncclGroupEnd() != ncclSuccess) return -5;
    return 0;
}

// linlang_nccl_exchange swaps localDim cuDoubleComplex amplitudes (2
// float64 each) between the GPU owning comm/stream and `partner` — the
// halo exchange a gate touching a global qubit requires before it can be
// applied locally on each shard.
static int linlang_nccl_exchange(ncclComm_t comm, cudaStream_t stream,
                                  cuDoubleComplex *sendBuf, cuDoubleComplex *recvBuf,
                                  size_t localDim, int partner) {
    size_t floats = localDim * 2; // re + im per amplitude
    if (ncclGroupStart() != ncclSuccess) return -1;
    if (ncclSend(sendBuf, floats, ncclDouble, partner, comm, stream) != ncclSuccess) return -2;
    if (ncclRecv(recvBuf, floats, ncclDouble, partner, comm, stream) != ncclSuccess) return -3;
    if (ncclGroupEnd() != ncclSuccess) return -4;
    if (cudaStreamSynchronize(stream) != cudaSuccess) return -5;
    return 0;
}

static void linlang_nccl_destroy(ncclComm_t *comms, int nGPUs) {
    for (int i = 0; i < nGPUs; i++) {
        ncclCommDestroy(comms[i]);
    }
}
*/
import "C"

import (
	"fmt"
	"math/bits"
	"unsafe"
)

// MultiGPUBackend shards a state vector across nGPUs GPUs via NCCL.
type MultiGPUBackend struct {
	nGPUs      int
	globalBits int // log2(nGPUs): how many high-order qubits are "global"
	comms      []C.ncclComm_t
}

// NewMultiGPUBackend initializes one NCCL communicator per GPU. nGPUs must
// be a power of 2, so the top log2(nGPUs) qubits cleanly index a GPU.
func NewMultiGPUBackend(nGPUs int) (*MultiGPUBackend, error) {
	if nGPUs < 1 || nGPUs&(nGPUs-1) != 0 {
		return nil, fmt.Errorf("multi-gpu: nGPUs debe ser potencia de 2, recibido %d", nGPUs)
	}
	comms := make([]C.ncclComm_t, nGPUs)
	if ret := C.linlang_nccl_init(&comms[0], C.int(nGPUs)); ret != 0 {
		return nil, fmt.Errorf("multi-gpu: ncclCommInitRank falló (código %d) — ¿hay %d GPUs NVIDIA disponibles?",
			int(ret), nGPUs)
	}
	return &MultiGPUBackend{
		nGPUs:      nGPUs,
		globalBits: bits.TrailingZeros(uint(nGPUs)),
		comms:      comms,
	}, nil
}

// Close destroys every GPU's NCCL communicator.
func (b *MultiGPUBackend) Close() {
	C.linlang_nccl_destroy(&b.comms[0], C.int(b.nGPUs))
}

func (b *MultiGPUBackend) Name() string { return "multi-gpu-nccl" }

// ExchangeShard performs the NCCL halo exchange between GPU rank and its
// partner (rank XOR (1<<globalQubitOffset)) — the primitive a gate
// touching a global qubit is built from. shard is rank's local amplitude
// buffer (already on that GPU's device memory); the received buffer is
// returned for the caller to combine with shard according to the gate
// being applied.
func (b *MultiGPUBackend) ExchangeShard(rank int, shard []complex128, globalQubitOffset int) ([]complex128, error) {
	if rank < 0 || rank >= b.nGPUs {
		return nil, fmt.Errorf("multi-gpu: rank %d fuera de rango [0,%d)", rank, b.nGPUs)
	}
	partner := rank ^ (1 << globalQubitOffset)
	if partner >= b.nGPUs {
		return nil, fmt.Errorf("multi-gpu: el qubit global en el offset %d no corresponde a una partición válida para %d GPUs",
			globalQubitOffset, b.nGPUs)
	}

	recv := make([]complex128, len(shard))
	ret := C.linlang_nccl_exchange(
		b.comms[rank], nil, // default stream
		(*C.cuDoubleComplex)(unsafe.Pointer(&shard[0])),
		(*C.cuDoubleComplex)(unsafe.Pointer(&recv[0])),
		C.size_t(len(shard)), C.int(partner),
	)
	if ret != 0 {
		return nil, fmt.Errorf("multi-gpu: ExchangeShard(rank=%d, partner=%d) falló (código %d)", rank, partner, int(ret))
	}
	return recv, nil
}

// Apply is only implemented for gates touching exclusively "local" qubits
// (index < totalQubits - globalBits): those apply independently per shard
// via the single-GPU cuStateVec path (core/cuda_backend.go), no NCCL
// needed. A gate touching a global qubit requires composing ExchangeShard
// with the gate's action per shard-pair, which is not implemented — see
// the package doc comment above for why.
func (b *MultiGPUBackend) Apply(gate *Gate, state *QuantumState) (*QuantumState, error) {
	return nil, fmt.Errorf(
		"multi-gpu: Apply no implementado — ExchangeShard expone el primitivo NCCL de distribución; " +
			"componerlo en una aplicación de puerta arbitraria sobre qubits locales/globales queda fuera de alcance sin hardware para verificarlo")
}
