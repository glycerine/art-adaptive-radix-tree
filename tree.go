package art

import (
	"fmt"
	"sync"
)

//go:generate greenpack

// Tree is a trie that implements
// the Adaptive Radix Tree (ART) algorithm [1].
// This provides both path compression
// and memory efficient child fanout.
//
// This ART implementation only allows
// one writer at a time.
// Benchmarking showed this to be the
// fastest approach in our applications.
//
// If a write is in progress, all readers
// are blocked. If a read is in
// progress, and no writer is waiting
// for the lock, then other readers
// can also RLock and proceed. The
// SkipLocking flag can be set to
// elide all locking.
//
// [1] "The Adaptive Radix Tree: ARTful
// Indexing for Main-Memory Databases"
// by Viktor Leis, Alfons Kemper, Thomas Neumann.
type Tree struct {
	Rwmut sync.RWMutex `msg:"-"`

	root *bnode
	size int64

	// Leafz is for serialization. You must
	// set leafByLeaf=false if you want to
	// automatically serialize a Tree when it is
	// a field in other structs. In that case,
	// the pre-save and post-load hooks will
	// use Leafz as a serialization buffer.
	// Otherwise Leafz is unused.
	//
	// Using Leafz may require more memory, since
	// the tree is fully serialized (temporarily)
	// into Leafz before writing anything to disk.
	// When leafByLeaf is true, the tree is
	// streamed to disk incrementally. See
	// saver.go and the TreeSaver and TreeLoader
	// for standalone save/load facilities.
	//
	// Only leaf nodes are serialized to disk.
	// This saves 20x space.
	Leafz []*Leaf `zid:"0"`

	// SkipLocking means do no internal
	// synchronization, because a higher
	// component is doing so.
	//
	// Warning when using SkipLocking:
	// the user's code _must_ synchronize (prevent
	// overlap) of readers and writers who access the Tree.
	// Under this setting, the Tree will not do locking.
	// (it does by default, with SkipLocking false).
	// Without synchronization, there will be data races,
	// lost data, and panic segfaults from torn reads.
	//
	// The easiest way to do this is with a sync.RWMutex.
	// One will be deployed for you if SkipLocking
	// defaults to false.
	SkipLocking bool `msg:"-"`
}

// used by tests; kind of a default value type.
type ByteSliceValue []byte

func NewArtTree() *Tree {
	return &Tree{}
}

// DeepSize enumerates all leaf nodes
// in order to compute the size. This is really only
// for testing. Prefer the cache based Size(),
// below, whenever possible.
func (t *Tree) DeepSize() (sz int) {

	for lf := range Ascend(t, nil, nil) {
		_ = lf
		sz++
	}
	return
}

// Size returns the number of keys
// (leaf nodes) stored in the tree.
func (t *Tree) Size() (sz int) {
	if t.SkipLocking {
		return int(t.size)
	}
	t.Rwmut.RLock()
	sz = int(t.size)
	t.Rwmut.RUnlock()
	return
}

func (t *Tree) String() string {
	sz := t.Size()
	if t.root == nil {
		return "empty tree"
	}
	return fmt.Sprintf("tree of size %v: ", sz) +
		t.root.FlatString(0, -1)
}

func (t *Tree) FlatString() string {
	sz := t.Size()
	if t.root == nil {
		return "empty tree"
	}

	return fmt.Sprintf("tree of size %v: \n", sz) +
		t.root.FlatString(0, -1)
}

// InsertX now copies the key to avoid bugs.
// The value is held by pointer in the interface.
// The x slice is not copied either.
func (t *Tree) InsertX(key Key, value any, x []byte) (updated bool) {

	key2 := Key(append([]byte{}, key...))
	lf := NewLeaf(key2, value, x)
	return t.InsertLeaf(lf)
}

// Insert makes a copy of key to avoid sharing bugs.
// The value is held by pointer in the interface.
func (t *Tree) Insert(key Key, value any) (updated bool) {

	// make a copy of key that we own, so
	// caller can alter/reuse without messing us up.
	// This was a frequent source of bugs, so
	// it is important. The benchmarks will crash
	// without it, for instance, since they
	// re-use key []byte memory alot.
	key2 := Key(append([]byte{}, key...))
	lf := NewLeaf(key2, value, nil)

	return t.InsertLeaf(lf)
}

// The *Leaf lf *must* own the lf.Key it holds.
// It cannot be shared. You must guarantee this,
// copying the slice if necessary.
func (t *Tree) InsertLeaf(lf *Leaf) (updated bool) {
	if t == nil {
		panic("t *Tree cannot be nil in InsertLeaf")
	}
	if !t.SkipLocking {
		t.Rwmut.Lock()
		defer t.Rwmut.Unlock()
	}

	var replacement *bnode

	if t.root == nil {
		// first node in tree
		t.size++
		t.root = bnodeLeaf(lf)
		return false
	}

	//vv("t.size = %v", t.size)
	replacement, updated = t.root.insert(lf, 0, t.root, t, nil)
	if replacement != nil {
		t.root = replacement
	}
	if !updated {
		t.size++
	}
	return
}

// FindGT returns the first element whose key
// is greater than the supplied key.
func (t *Tree) FindGT(key Key) (val any, found bool) {
	var lf *Leaf
	lf, found = t.Find(GT, key)
	if found && lf != nil {
		val = lf.Value
	}
	return
}

// FindGTE returns the first element whose key
// is greater than, or equal to, the supplied key.
func (t *Tree) FindGTE(key Key) (val any, found bool) {
	var lf *Leaf
	lf, found = t.Find(GTE, key)
	if found && lf != nil {
		val = lf.Value
	}
	return
}

// FindGT returns the first element whose key
// is less than the supplied key.
func (t *Tree) FindLT(key Key) (val any, found bool) {
	var lf *Leaf
	lf, found = t.Find(LT, key)
	if found && lf != nil {
		val = lf.Value
	}
	return
}

// FindLTE returns the first element whose key
// is less-than-or-equal to the supplied key.
func (t *Tree) FindLTE(key Key) (val any, found bool) {
	var lf *Leaf
	lf, found = t.Find(LTE, key)
	if found && lf != nil {
		val = lf.Value
	}
	return
}

// FindExact returns the element whose key
// matches the supplied key.
func (t *Tree) FindExact(key Key) (val any, found bool) {
	var lf *Leaf
	lf, found = t.Find(Exact, key)
	if found && lf != nil {
		val = lf.Value
	}
	return
}

// FirstLeaf returns the first leaf in the Tree.
func (t *Tree) FirstLeaf() (lf *Leaf, found bool) {
	return t.Find(GTE, nil)
}

// FirstLeaf returns the last leaf in the Tree.
func (t *Tree) LastLeaf() (lf *Leaf, found bool) {
	return t.Find(LTE, nil)
}

// Find allows GTE, GT, LTE, LT, and Exact searches.
//
// GTE: find a leaf greater-than-or-equal to key;
// the smallest such key.
//
// GT: find a leaf strictly greater-than key;
// the smallest such key.
//
// LTE: find a leaf less-than-or-equal to key;
// the largest such key.
//
// LT: find a leaf less-than key; the
// largest such key.
//
// Exact: find leaf whose key matches the supplied
// key exactly. This is the default. It acts
// like a hash table. A key can only be stored
// once in the tree. (It is not a multi-map
// in the C++ STL sense).
//
// If key is nil, then GTE and GT return
// the first leaf in the tree, while LTE
// and LT return the last leaf in the tree.
func (t *Tree) Find(smod SearchModifier, key Key) (lf *Leaf, found bool) {
	if !t.SkipLocking {
		t.Rwmut.RLock()
		defer t.Rwmut.RUnlock()
	}
	if t.root == nil {
		return
	}
	if len(key) == 0 && t.size == 1 {
		// nil query asks for first leaf, or last, depending.
		// here it is the same.
		return t.root.leaf, true
	}
	var b *bnode
	switch smod {
	case GTE, GT:
		b, found, _, _ = t.root.getGTE(key, 0, smod, t.root, t, 0, false, 0)
	case LTE, LT:
		b, found, _, _ = t.root.getLTE(key, 0, smod, t.root, t, 0, false, 0)
	default:
		b, found, _, _ = t.root.get(key, 0, t.root)
	}
	if b != nil {
		lf = b.leaf
	}
	return
}

type SearchModifier int

const (
	// Exact is the default.
	Exact SearchModifier = 0 // exact matches only; like a hash table
	GTE   SearchModifier = 1 // greater than or equal to this key.
	LTE   SearchModifier = 2 // less than or equal to this key.
	GT    SearchModifier = 3 // strictly greater than this key.
	LT    SearchModifier = 4 // strictly less than this key.
)

func (smod SearchModifier) String() string {
	switch smod {
	case Exact:
		return "Exact"
	case GTE:
		return "GTE"
	case LTE:
		return "LTE"
	case GT:
		return "GT"
	case LT:
		return "LT"
	}
	panic(fmt.Sprintf("unknown smod '%v'", int(smod)))
}

// Remove deletes the key from the Tree.
func (t *Tree) Remove(key Key) (deleted bool, value any) {

	if !t.SkipLocking {
		t.Rwmut.Lock()
		defer t.Rwmut.Unlock()
	}

	var deletedNode *bnode
	if t.root == nil {
		return
	}

	for {
		deleted, deletedNode = t.root.del(key, 0, t.root, func(rn *bnode) {
			t.root = rn
		})
		if deleted {
			value = deletedNode.leaf.Value
			t.size--
		}
		return deleted, value
	}
}

// IsEmpty returns true iff the Tree is empty.
func (t *Tree) IsEmpty() (empty bool) {
	if t.SkipLocking {
		return t.root == nil
	}
	t.Rwmut.RLock()
	empty = t.root == nil
	t.Rwmut.RUnlock()
	return
}

// Iterator in range (start, end].
// Iterator is concurrently safe, but doesn't guarantee to provide consistent
// snapshot of the tree state.
func (t *Tree) Iterator(start, end []byte) *iterator {
	return &iterator{
		tree:      t,
		cursor:    start,
		terminate: end,
	}
}

// At(i) lets us think of the tree as an
// array, returning the i-th leaf
// from the sorted leaf nodes, using
// an efficient O(log N) time algorithm.
// Here N is the size or count of elements
// stored in the tree.
//
// At() uses the counted B-tree approach
// described by Simon Tatham of PuTTY fame[1].
// [1] Reference:
// https://www.chiark.greenend.org.uk/~sgtatham/algorithms/cbtree.html
func (t *Tree) At(i int) (lf *Leaf, ok bool) {
	if t.SkipLocking {
		lf, ok = t.root.at(i)
		return
	}
	t.Rwmut.RLock()
	lf, ok = t.root.at(i)
	t.Rwmut.RUnlock()
	return
}
