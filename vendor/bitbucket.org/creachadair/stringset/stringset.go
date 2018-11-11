// Package stringset implements a lightweight (finite) set-of-string type
// based on Go's built-in map.  A Set provides some convenience methods for
// common set operations.
//
// A nil Set is ready for use as an empty set.  The basic set methods (Diff,
// Intersect, Union, IsSubset, Map, Choose, Partition) do not mutate their
// arguments.  There are also mutating operations (Add, Discard, Pop, Remove,
// Update) that modify their receiver in-place.
//
// A Set can also be traversed and modified using the normal map operations.
// Being a map, a Set is not safe for concurrent access by multiple goroutines
// unless all the concurrent accesses are reads.
package stringset

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func toString(x string) string {
	return strconv.Quote(x)
}

// A Set represents a set of string values.  A nil Set is a valid
// representation of an empty set.
type Set map[string]struct{}

// byElement satisfies sort.Interface to order values of type string.
type byElement []string

func (e byElement) Len() int      { return len(e) }
func (e byElement) Swap(i, j int) { e[i], e[j] = e[j], e[i] }
func (e byElement) Less(i, j int) bool {
	return e[i] < e[j]
}

// String implements the fmt.Stringer interface.  It renders s in standard set
// notation, e.g., ø for an empty set, {a, b, c} for a nonempty one.
func (s Set) String() string {
	if s.Empty() {
		return "ø"
	}
	elts := make([]string, len(s))
	for i, elt := range s.Elements() {
		elts[i] = toString(elt)
	}
	return "{" + strings.Join(elts, ", ") + "}"
}

// New returns a new set containing exactly the specified elements.
// Returns a non-nil empty Set if no elements are specified.
func New(elts ...string) Set {
	set := make(Set, len(elts))
	for _, elt := range elts {
		set[elt] = struct{}{}
	}
	return set
}

// NewSize returns a new empty set pre-sized to hold at least n elements.
// This is equivalent to make(Set, n) and will panic if n < 0.
func NewSize(n int) Set { return make(Set, n) }

// Len returns the number of elements in s.
func (s Set) Len() int { return len(s) }

// Elements returns an ordered slice of the elements in s.
func (s Set) Elements() []string {
	elts := s.Unordered()
	sort.Sort(byElement(elts))
	return elts
}

// Unordered returns an unordered slice of the elements in s.
func (s Set) Unordered() []string {
	if len(s) == 0 {
		return nil
	}
	elts := make([]string, 0, len(s))
	for elt := range s {
		elts = append(elts, elt)
	}
	return elts
}

// Clone returns a new Set distinct from s, containing the same elements.
func (s Set) Clone() Set {
	var c Set
	c.Update(s)
	return c
}

// ContainsAny reports whether s contains one or more of the given elements.
// It is equivalent in meaning to
//   s.Intersects(stringset.New(elts...))
// but does not construct an intermediate set.
func (s Set) ContainsAny(elts ...string) bool {
	for _, key := range elts {
		if _, ok := s[key]; ok {
			return true
		}
	}
	return false
}

// Contains reports whether s contains (all) the given elements.
// It is equivalent in meaning to
//   New(elts...).IsSubset(s)
// but does not construct an intermediate set.
func (s Set) Contains(elts ...string) bool {
	for _, elt := range elts {
		if _, ok := s[elt]; !ok {
			return false
		}
	}
	return true
}

// IsSubset reports whether s is a subset of s2, s ⊆ s2.
func (s Set) IsSubset(s2 Set) bool {
	if s.Empty() {
		return true
	} else if len(s) > len(s2) {
		return false
	}
	for k := range s {
		if _, ok := s2[k]; !ok {
			return false
		}
	}
	return true
}

// Equals reports whether s is equal to s2, having exactly the same elements.
func (s Set) Equals(s2 Set) bool { return len(s) == len(s2) && s.IsSubset(s2) }

// Empty reports whether s is empty.
func (s Set) Empty() bool { return len(s) == 0 }

// Intersects reports whether the intersection s ∩ s2 is non-empty, without
// explicitly constructing the intersection.
func (s Set) Intersects(s2 Set) bool {
	a, b := s, s2
	if len(b) < len(a) {
		a, b = b, a // Iterate over the smaller set
	}
	for k := range a {
		if _, ok := b[k]; ok {
			return true
		}
	}
	return false
}

// Union constructs the union s ∪ s2.
func (s Set) Union(s2 Set) Set {
	if s.Empty() {
		return s2
	} else if s2.Empty() {
		return s
	}
	set := make(Set)
	for k := range s {
		set[k] = struct{}{}
	}
	for k := range s2 {
		set[k] = struct{}{}
	}
	return set
}

// Intersect constructs the intersection s ∩ s2.
func (s Set) Intersect(s2 Set) Set {
	if s.Empty() || s2.Empty() {
		return nil
	}
	set := make(Set)
	for k := range s {
		if _, ok := s2[k]; ok {
			set[k] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// Diff constructs the set difference s \ s2.
func (s Set) Diff(s2 Set) Set {
	if s.Empty() || s2.Empty() {
		return s
	}
	set := make(Set)
	for k := range s {
		if _, ok := s2[k]; !ok {
			set[k] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// SymDiff constructs the symmetric difference s ∆ s2.
// It is equivalent in meaning to (s ∪ s2) \ (s ∩ s2).
func (s Set) SymDiff(s2 Set) Set {
	return s.Union(s2).Diff(s.Intersect(s2))
}

// Update adds the elements of s2 to *s in-place, and reports whether anything
// was added.
// If *s == nil and s2 ≠ ø, a new set is allocated that is a copy of s2.
func (s *Set) Update(s2 Set) bool {
	in := len(*s)
	if *s == nil && len(s2) > 0 {
		*s = make(Set)
	}
	for k := range s2 {
		(*s)[k] = struct{}{}
	}
	return len(*s) != in
}

// Add adds the specified elements to *s in-place and reports whether anything
// was added.  If *s == nil, a new set equivalent to New(ss...) is stored in *s.
func (s *Set) Add(ss ...string) bool {
	in := len(*s)
	if *s == nil {
		*s = make(Set)
	}
	for _, key := range ss {
		(*s)[key] = struct{}{}
	}
	return len(*s) != in
}

// Remove removes the elements of s2 from s in-place and reports whether
// anything was removed.
//
// Equivalent to s = s.Diff(s2), but does not allocate a new set.
func (s Set) Remove(s2 Set) bool {
	in := s.Len()
	if !s.Empty() {
		for k := range s2 {
			delete(s, k)
		}
	}
	return s.Len() != in
}

// Discard removes the elements of elts from s in-place and reports whether
// anything was removed.
//
// Equivalent to s.Remove(New(elts...)), but does not allocate an intermediate
// set for ss.
func (s Set) Discard(elts ...string) bool {
	in := s.Len()
	if !s.Empty() {
		for _, elt := range elts {
			delete(s, elt)
		}
	}
	return s.Len() != in
}

// Index returns the first offset of needle in elts, if it occurs; otherwise -1.
func Index(needle string, elts []string) int {
	for i, elt := range elts {
		if elt == needle {
			return i
		}
	}
	return -1
}

// Contains reports whether v contains s, for v having type Set, []string,
// map[string]T, or Keyer. It returns false if v's type does not have one of
// these forms.
func Contains(v interface{}, s string) bool {
	switch t := v.(type) {
	case []string:
		return Index(s, t) >= 0
	case Set:
		return t.Contains(s)
	case Keyer:
		return Index(s, t.Keys()) >= 0
	}
	if m := reflect.ValueOf(v); m.IsValid() && m.Kind() == reflect.Map && m.Type().Key() == refType {
		return m.MapIndex(reflect.ValueOf(s)).IsValid()
	}
	return false
}

// A Keyer implements a Keys method that returns the keys of a collection such
// as a map or a Set.
type Keyer interface {
	// Keys returns the keys of the receiver, which may be nil.
	Keys() []string
}

var refType = reflect.TypeOf((*string)(nil)).Elem()

// FromKeys returns a Set of strings from v, which must either be a string,
// a []string, a map[string]T, or a Keyer. It returns nil if v's type does
// not have one of these forms.
func FromKeys(v interface{}) Set {
	var result Set
	switch t := v.(type) {
	case string:
		return New(t)
	case []string:
		for _, key := range t {
			result.Add(key)
		}
		return result
	case map[string]struct{}: // includes Set
		for key := range t {
			result.Add(key)
		}
		return result
	case Keyer:
		for _, key := range t.Keys() {
			result.Add(key)
		}
		return result
	case nil:
		return nil
	}
	m := reflect.ValueOf(v)
	if m.Kind() != reflect.Map || m.Type().Key() != refType {
		return nil
	}
	for _, key := range m.MapKeys() {
		result.Add(key.Interface().(string))
	}
	return result
}

// FromValues returns a Set of the values from v, which has type map[T]string.
// Returns the empty set if v does not have a type of this form.
func FromValues(v interface{}) Set {
	if t := reflect.TypeOf(v); t == nil || t.Kind() != reflect.Map || t.Elem() != refType {
		return nil
	}
	var set Set
	m := reflect.ValueOf(v)
	for _, key := range m.MapKeys() {
		set.Add(m.MapIndex(key).Interface().(string))
	}
	return set
}
