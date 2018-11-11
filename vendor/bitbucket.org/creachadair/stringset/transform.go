package stringset

// Map returns the Set that results from applying f to each element of s.
func (s Set) Map(f func(string) string) Set {
	var out Set
	for k := range s {
		out.Add(f(k))
	}
	return out
}

// Each applies f to each element of s.
func (s Set) Each(f func(string)) {
	for k := range s {
		f(k)
	}
}

// Select returns the subset of s for which f returns true.
func (s Set) Select(f func(string) bool) Set {
	var out Set
	for k := range s {
		if f(k) {
			out.Add(k)
		}
	}
	return out
}

// Partition returns two disjoint sets, yes containing the subset of s for
// which f returns true and no containing the subset for which f returns false.
func (s Set) Partition(f func(string) bool) (yes, no Set) {
	for k := range s {
		if f(k) {
			yes.Add(k)
		} else {
			no.Add(k)
		}
	}
	return
}

// Choose returns an element of s for which f returns true, if one exists.  The
// second result reports whether such an element was found.
// If f == nil, chooses an arbitrary element of s.
func (s Set) Choose(f func(string) bool) (string, bool) {
	if f == nil {
		for k := range s {
			return k, true
		}
	}
	for k := range s {
		if f(k) {
			return k, true
		}
	}
	return "", false
}

// Pop removes and returns an element of s for which f returns true, if one
// exists (essentially Choose + Discard).  The second result reports whether
// such an element was found.  If f == nil, pops an arbitrary element of s.
func (s Set) Pop(f func(string) bool) (string, bool) {
	if v, ok := s.Choose(f); ok {
		delete(s, v)
		return v, true
	}
	return "", false
}

// Count returns the number of elements of s for which f returns true.
func (s Set) Count(f func(string) bool) (n int) {
	for k := range s {
		if f(k) {
			n++
		}
	}
	return
}
