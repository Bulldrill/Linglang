//go:build cuda

package core

// CUDASimBackend: GPU-accelerated state-vector simulator via NVIDIA
// cuQuantum's cuStateVec library (https://docs.nvidia.com/cuda/cuquantum).
//
// Excluded from the default build by the "cuda" build tag above: compiling
// this file requires the CUDA toolkit and libcustatevec installed, neither
// of which is available in this development environment. It has NOT been
// compiled or tested here — build with `go build -tags cuda` on a machine
// with the NVIDIA cuQuantum SDK installed, and review the cgo calls below
// against your installed cuStateVec version before relying on it.
//
// Unlike SuperconductorBackend/TrappedIonBackend, this backend is still a
// full software (just GPU-resident) simulator: Apply/Measure on an
// arbitrary amplitude vector remain physically meaningful, exactly as for
// SimulatorBackend — only the O(2^n) matrix-vector product is offloaded to
// the GPU via custatevecApplyMatrix.

/*
#cgo LDFLAGS: -lcudart -lcustatevec
#include <cuda_runtime.h>
#include <custatevec.h>
#include <stdlib.h>

static custatevecHandle_t linlang_handle = NULL;

static int linlang_cuda_init() {
    if (linlang_handle != NULL) return 0;
    return custatevecCreate(&linlang_handle) == CUSTATEVEC_STATUS_SUCCESS ? 0 : -1;
}

// linlang_apply_matrix copies sv (nAmps cuDoubleComplex) and matrix
// (nAmps*nAmps cuDoubleComplex) to device memory, applies the gate via
// custatevecApplyMatrix over nIndexBits = log2(nAmps), and copies the
// result back into sv in place.
static int linlang_apply_matrix(cuDoubleComplex *sv, int nAmps,
                                 const cuDoubleComplex *matrix, int nIndexBits) {
    if (linlang_cuda_init() != 0) return -1;

    cuDoubleComplex *dSv = NULL, *dMatrix = NULL;
    size_t svBytes = (size_t)nAmps * sizeof(cuDoubleComplex);
    size_t matBytes = (size_t)nAmps * (size_t)nAmps * sizeof(cuDoubleComplex);

    if (cudaMalloc((void**)&dSv, svBytes) != cudaSuccess) return -2;
    if (cudaMalloc((void**)&dMatrix, matBytes) != cudaSuccess) { cudaFree(dSv); return -3; }
    cudaMemcpy(dSv, sv, svBytes, cudaMemcpyHostToDevice);
    cudaMemcpy(dMatrix, matrix, matBytes, cudaMemcpyHostToDevice);

    int32_t targets[32];
    for (int i = 0; i < nIndexBits; i++) targets[i] = i;

    size_t workspaceSize = 0;
    custatevecApplyMatrixGetWorkspaceSize(
        linlang_handle, CUDA_C_64F, nIndexBits, dMatrix, CUDA_C_64F,
        CUSTATEVEC_MATRIX_LAYOUT_ROW, 0, nIndexBits, 0,
        CUSTATEVEC_COMPUTE_64F, &workspaceSize);

    void *workspace = NULL;
    if (workspaceSize > 0) cudaMalloc(&workspace, workspaceSize);

    custatevecStatus_t st = custatevecApplyMatrix(
        linlang_handle, dSv, CUDA_C_64F, nIndexBits,
        dMatrix, CUDA_C_64F, CUSTATEVEC_MATRIX_LAYOUT_ROW, 0,
        targets, nIndexBits, NULL, 0, NULL,
        CUSTATEVEC_COMPUTE_64F, workspace, workspaceSize);

    cudaMemcpy(sv, dSv, svBytes, cudaMemcpyDeviceToHost);

    if (workspace) cudaFree(workspace);
    cudaFree(dSv);
    cudaFree(dMatrix);
    return st == CUSTATEVEC_STATUS_SUCCESS ? 0 : -4;
}
*/
import "C"

import (
	"fmt"
	"math"
	"unsafe"
)

type CUDASimBackend struct{}

func init() {
	cudaBackendFactory = func() (QuantumBackend, error) {
		if C.linlang_cuda_init() != 0 {
			return nil, fmt.Errorf("cuda: no se pudo inicializar el handle de cuStateVec (¿hay una GPU NVIDIA disponible?)")
		}
		return &CUDASimBackend{}, nil
	}
}

func (b *CUDASimBackend) Name() string { return "gpu-custatevec" }

// Apply offloads the matrix-vector product U|ψ⟩ to the GPU via
// custatevecApplyMatrix.
func (b *CUDASimBackend) Apply(gate *Gate, state *QuantumState) (*QuantumState, error) {
	if gate == nil || state == nil {
		return nil, fmt.Errorf("cuda: gate y estado no pueden ser nil")
	}
	n := len(state.Amplitudes)
	if len(gate.Matrix) != n {
		return nil, fmt.Errorf("cuda: dimensión de la puerta (%d) no coincide con el estado (%d)", len(gate.Matrix), n)
	}
	nIndexBits := int(math.Round(math.Log2(float64(n))))
	if 1<<nIndexBits != n {
		return nil, fmt.Errorf("cuda: la dimensión del estado (%d) debe ser potencia de 2", n)
	}

	sv := make([]complex128, n)
	copy(sv, state.Amplitudes)
	matrix := make([]complex128, n*n)
	for i, row := range gate.Matrix {
		copy(matrix[i*n:(i+1)*n], row)
	}

	ret := C.linlang_apply_matrix(
		(*C.cuDoubleComplex)(unsafe.Pointer(&sv[0])), C.int(n),
		(*C.cuDoubleComplex)(unsafe.Pointer(&matrix[0])), C.int(nIndexBits),
	)
	if ret != 0 {
		return nil, fmt.Errorf("cuda: custatevecApplyMatrix falló (código %d)", int(ret))
	}
	return NewQuantumState(gate.Codomain, sv), nil
}

// Measure copies the (already GPU-evolved) amplitudes back to the host and
// reuses the same Born-rule argmax as SimulatorBackend — cheap relative to
// gate application, and keeps measurement semantics identical across
// backends.
func (b *CUDASimBackend) Measure(state *QuantumState) (int, []float64, error) {
	if state == nil {
		return 0, nil, fmt.Errorf("cuda: estado no puede ser nil")
	}
	outcome, probs := state.Measure()
	return outcome, probs, nil
}

func (b *CUDASimBackend) SupportedGates() []string {
	return []string{"H", "X", "Y", "Z", "I", "CNOT"}
}

func (b *CUDASimBackend) Topology() Topology {
	return Topology{} // fully connected: still a simulator, just GPU-resident
}

func (b *CUDASimBackend) NoiseModel() NoiseModel {
	return NoNoise()
}
