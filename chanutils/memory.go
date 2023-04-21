package chanutils

// Ptr returns the pointer of the given value. This is useful in instances
// where a function returns the value, but a pointer is wanted. Without this,
// then an intermediate variable is needed.
func Ptr[T any](v T) *T {
	return &v
}

// ByteArray is a type constraint for type that reduces down to a fixed sized
// array.
type ByteArray interface {
	~[32]byte
}

// ByteSlice takes a byte array, and returns a slice. This is useful when a
// function returns an array, but a slice is wanted. Without this, then an
// intermediate variable is needed.
func ByteSlice[T ByteArray](v T) []byte {
	return v[:]
}

// ToArray takes a byte slice, and returns an array. This is useful when a
// fixed sized array is needed and the byte slice is known to be of the correct
// size.
func ToArray[T ByteArray](v []byte) T {
	var arr T
	copy(arr[:], v)
	return arr
}