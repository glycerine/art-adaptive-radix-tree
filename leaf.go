package art

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	//"github.com/glycerine/greenpack/msgp"
)

var _ = sync.RWMutex{}

const (
	XTypBytes int = 0
)

type TestBytes struct {
	Slc []byte `zid:"0"`
}

//go:generate greenpack

// A simple wrapper header on all msgpack
// messages; has the length and the bytes.
// Allows us length delimited messages;
// with length knowledge up front.
type ByteSlice []byte

type Key []byte

type Leaf struct {
	//rwmut artlock
	//rwmut sync.RWMutex

	Key   Key         `zid:"0"`
	Value interface{} `msg:"-"`

	// What is XTyp? It could be...
	// the length of X that follows... or a
	// seqpos... or a database foreign key... or just empty.
	// All are possible. XTyp and X are
	// application specific.
	// We currently use them for serialization
	// (greenpack/msgpack based) save-to-sisk support.
	// The saver.go and saver_test.go file
	// use them.

	XTyp int    `zid:"2"`
	X    []byte `zid:"3"`

	Keybyte byte `zid:"1"`
}

func (n *Leaf) depth() int {
	return len(n.Key)
}
func (n *Leaf) clone() (c *Leaf) {
	c = &Leaf{
		Key:     append([]byte{}, n.Key...),
		Value:   n.Value, // shared interface (pointer to Value)
		XTyp:    n.XTyp,
		X:       append([]byte{}, n.X...),
		Keybyte: n.Keybyte,
	}
	return c
}

func (lf *Leaf) DepthString(depth int, prior []byte, recurse int) string {
	rep := strings.Repeat("    ", depth)

	valstr := ""
	switch x := (lf.Value).(type) {
	case []byte:
		valstr = string(x)
	case ByteSliceValue:
		valstr = string(x)
	case int: // tests use
		valstr = fmt.Sprintf("int value: '%v'", x)
	case *TestBytes:
		valstr = string(x.Slc)
	default:
		valstr = fmt.Sprintf("opaque %T", lf.Value)
		//panic(fmt.Sprintf("how to turn %T into string?", lf.Value))
	}

	return fmt.Sprintf(`%[1]v Leaf{
%[1]v       key: "%[2]v" (len %[3]v),
%[1]v     value: "%[4]v",
%[1]v }
`, rep,
		string(lf.Key), len(lf.Key),
		valstr,
	)
}

func (lf *Leaf) PreSaveHook() {
	if lf.Value == nil {
		return
	}
	switch x := lf.Value.(type) {
	case ByteSliceValue:
		lf.XTyp = XTypBytes
		lf.X = []byte(x)
	case []byte:
		lf.XTyp = XTypBytes
		lf.X = x
	default:
		panic("add a case here for your data type")
	}
}

func (lf *Leaf) PostLoadHook() {
	switch lf.XTyp {
	case XTypBytes:
		lf.Value = ByteSliceValue(lf.X)
	default:
		panic("add a case here for your data type")
	}
}

func NewLeaf(key Key, v any, x []byte) *Leaf {
	return &Leaf{
		Key:   key,
		Value: v,
		X:     x,
	}
}

func (lf *Leaf) Kind() Kind {
	return Leafy
}

func (lf *Leaf) insert(other *Leaf, depth int, selfb *bnode, tree *Tree, par *Inner) (value *bnode, updated bool) {

	if lf == other {
		// due to restarts (now elided though),
		// we might be trying to put ourselves in
		// the tree twice.
		return selfb, false
	}

	if other.cmp(lf.Key) {
		return bnodeLeaf(other), true
	}

	longestPrefix := comparePrefix(lf.Key, other.Key, depth)
	//vv("longestPrefix = %v; lf.Key='%v', other.key='%v', depth=%v", longestPrefix, string(lf.Key), string(other.Key), depth)
	n4 := &node4{}
	nn := &Inner{
		Node: n4,

		// keep commented out path stuff for debugging!
		//path: append([]byte{}, lf.Key[:depth+longestPrefix]...),
		SubN: 2,
	}
	//vv("assigned path '%v' to %p", string(nn.path), nn)
	if longestPrefix > 0 {
		nn.compressed = append([]byte{}, lf.Key[depth:depth+longestPrefix]...)
	}
	//vv("leaf insert: lef nn.PrefixLen = %v (longestPrefix)", nn.PrefixLen)

	child0key := lf.Key.At(depth + longestPrefix)
	child1key := other.Key.At(depth + longestPrefix)

	//vv("child0key = 0x%x; lf.Key = '%v' (len %v); depth=%v; longestPrefix=%v; depth+longestPrefix=%v", child0key, string(lf.Key), len(lf.Key), depth, longestPrefix, depth+longestPrefix)

	nn.Node.addChild(child0key, bnodeLeaf(lf))
	nn.Node.addChild(child1key, bnodeLeaf(other))

	selfb.isLeaf = false
	selfb.inner = nn
	return selfb, false
}

func (lf *Leaf) del(key Key, depth int, selfb *bnode, parentUpdate func(*bnode)) (deleted bool, deletedNode *bnode) {

	if !lf.cmpUnlocked(key) {
		return false, nil
	}

	parentUpdate(nil)

	return true, selfb
}

func (lf *Leaf) get(key Key, i int, selfb *bnode) (value *bnode, found bool, dir direc, id int) {
	cmp := bytes.Compare(key, lf.Key)
	//pp("top of Leaf get, cmp = %v from lf.Key='%v'; key='%v'", cmp, string(lf.Key), string(key))
	//defer func() {
	//pp("Leaf '%v' returns found=%v, dir=%v", string(lf.Key), found, dir)
	//}()

	// return ourselves even if not exact match, to avoid
	// a second recursive descent on GTE, for example.
	return selfb, cmp == 0, direc(cmp), 0
}

func (lf *Leaf) addPrefixBefore(node *Inner, key byte) {
	// Leaf does not store prefixes, only Inner.
}

func (lf *Leaf) isLeaf() bool {
	return true
}

func (lf *Leaf) String() string {
	//return fmt.Sprintf("leaf[%q]", string(lf.Key))
	return lf.FlatString(0, 0)
}

// use by get
func (lf *Leaf) cmp(other []byte) (equal bool) {
	return bytes.Compare(lf.Key, other) == 0
}

// use by del, already holding Lock
func (lf *Leaf) cmpUnlocked(other []byte) (equal bool) {
	equal = bytes.Compare(lf.Key, other) == 0
	return
}

func (n *Leaf) FlatString(depth int, recurse int) (s string) {
	rep := strings.Repeat("    ", depth)
	return fmt.Sprintf(`%[1]v %p leaf: key '%v' (len %v)%v`,
		rep,
		n,
		viznlString(n.Key),
		len(n.Key),
		"\n",
	)
}

func (n *Leaf) str() string {
	return string(n.Key)
}
