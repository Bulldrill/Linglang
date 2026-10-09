package core

import "testing"

func benchVectors(dim int) (*Vector, *Vector) {
	sp := NewSpace("Bench", make([]string, dim))
	for i := range sp.Dimensions {
		sp.Dimensions[i] = "d"
	}
	a := make([]float64, dim)
	b := make([]float64, dim)
	for i := range a {
		a[i] = float64(i)
		b[i] = float64(dim - i)
	}
	return NewVector(sp, a), NewVector(sp, b)
}

func BenchmarkVectorAdd(b *testing.B) {
	for _, dim := range []int{4, 64, 1024} {
		v1, v2 := benchVectors(dim)
		b.Run(itoa(dim), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = v1.Add(v2)
			}
		})
	}
}

func BenchmarkVectorDot(b *testing.B) {
	for _, dim := range []int{4, 64, 1024} {
		v1, v2 := benchVectors(dim)
		b.Run(itoa(dim), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = v1.Dot(v2)
			}
		})
	}
}

func BenchmarkVectorNorm(b *testing.B) {
	for _, dim := range []int{4, 64, 1024} {
		v1, _ := benchVectors(dim)
		b.Run(itoa(dim), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = v1.Norm()
			}
		})
	}
}

func BenchmarkVectorScale(b *testing.B) {
	for _, dim := range []int{4, 64, 1024} {
		v1, _ := benchVectors(dim)
		b.Run(itoa(dim), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = v1.Scale(2.5)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
