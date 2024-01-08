package util

import (
	"container/list"
	"fmt"
	"slices"
	"sync"
)

// Ring is a bidirectional loop linked list.
type Ring struct {
	mtx  sync.Mutex
	data *list.List
}

// NewRing creates a new ring.
func NewRing() *Ring {
	return &Ring{
		data: list.New(),
	}
}

// InsertAll inserts all values into the ring.
func (s *Ring) InsertAll(value []uint64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, v := range value {
		s.insert(v)
	}
}

// Insert inserts a value into the ring.
func (s *Ring) Insert(value uint64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.insert(value)
}

// Remove removes a value from the ring.
func (s *Ring) Remove(value uint64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.remove(value)
}

// Find finds a value in the ring.
func (s *Ring) Find(value uint64) *list.Element {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.find(value)
}

// Neighbors returns the neighbors of a value in the ring.
func (s *Ring) Neighbors(value uint64) (uint64, uint64, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	element := s.find(value)
	var left, right *list.Element

	if element != nil {
		if prev := element.Prev(); prev != nil {
			left = prev
		} else {
			left = s.data.Back()
		}

		if next := element.Next(); next != nil {
			right = next
		} else {
			right = s.data.Front()
		}

		return left.Value.(uint64), right.Value.(uint64), nil
	}

	return 0, 0, fmt.Errorf("not found")
}

// GetAll returns all values in the ring.
func (s *Ring) GetAll() []uint64 {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	result := make([]uint64, 0, s.data.Len())

	for element := s.data.Front(); element != nil; element = element.Next() {
		result = append(result, element.Value.(uint64))
	}

	return result
}

// Clear clears the ring.
func (s *Ring) Clear() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.data.Init()
}

// Sort sorts elements in the ring.
func (s *Ring) Sort() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	values := make([]uint64, s.data.Len())
	i := 0

	for element := s.data.Front(); element != nil; element = element.Next() {
		values[i] = element.Value.(uint64)
		i++
	}

	slices.Sort(values)
	s.data.Init()
	for _, value := range values {
		s.data.PushBack(value)
	}
}

// insert inserts a value into the ring.
func (s *Ring) insert(value uint64) {
	s.data.PushBack(value)
}

// find finds a value in the ring.
func (s *Ring) find(value uint64) *list.Element {
	for element := s.data.Front(); element != nil; element = element.Next() {
		if element.Value.(uint64) == value {
			return element
		}
	}
	return nil
}

// remove removes a value from the ring.
func (s *Ring) remove(value uint64) {
	for element := s.data.Front(); element != nil; element = element.Next() {
		if element.Value.(uint64) == value {
			s.data.Remove(element)
			break
		}
	}
}
