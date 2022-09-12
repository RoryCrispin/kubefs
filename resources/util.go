package resources

import (
	"hash/fnv"
)

// hash generates a uint64 hash from a given string.
// It's useful for generating stable inode numbers.
func hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
