package goforge

func PtrValue[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}