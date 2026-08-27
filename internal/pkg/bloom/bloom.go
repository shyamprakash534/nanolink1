package bloom

import (
	"hash/fnv"
	"math"
	"sync"
)

// BloomFilter is an in-memory space-efficient probabilistic filter
type BloomFilter struct {
	mu        sync.RWMutex
	bitset    []uint64
	m         uint64 // number of bits
	k         uint64 // number of hash functions
	count     uint64
}

// NewBloomFilter creates a Bloom filter optimized for expectedElements and target falsePositiveRate
func NewBloomFilter(expectedElements uint64, falsePositiveRate float64) *BloomFilter {
	if falsePositiveRate <= 0 {
		falsePositiveRate = 0.01
	}
	if expectedElements == 0 {
		expectedElements = 1000000
	}

	// m = - (n * ln(p)) / (ln(2)^2)
	m := uint64(math.Ceil(-1 * float64(expectedElements) * math.Log(falsePositiveRate) / math.Pow(math.Log(2), 2)))
	// k = (m / n) * ln(2)
	k := uint64(math.Round((float64(m) / float64(expectedElements)) * math.Log(2)))
	if k == 0 {
		k = 1
	}

	words := (m + 63) / 64
	return &BloomFilter{
		bitset: make([]uint64, words),
		m:      m,
		k:      k,
	}
}

func (bf *BloomFilter) hash(data string, seed uint64) uint64 {
	h := fnv.New64a()
	h.Write([]byte(data))
	v := h.Sum64()
	return (v ^ (seed * 0x5bd1e9955bd1e995)) % bf.m
}

// Add inserts an element into the Bloom filter
func (bf *BloomFilter) Add(element string) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	for i := uint64(0); i < bf.k; i++ {
		idx := bf.hash(element, i)
		word := idx / 64
		bit := idx % 64
		bf.bitset[word] |= (1 << bit)
	}
	bf.count++
}

// Contains checks if an element might be in the set
func (bf *BloomFilter) Contains(element string) bool {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	for i := uint64(0); i < bf.k; i++ {
		idx := bf.hash(element, i)
		word := idx / 64
		bit := idx % 64
		if (bf.bitset[word] & (1 << bit)) == 0 {
			return false // Definitely does not exist
		}
	}
	return true // May exist
}
