package art

import (
	"bufio"
	"bytes"
	"fmt"
	"time"
	//"bytes"
	//"encoding/binary"
	"github.com/stretchr/testify/assert"
	//"github.com/tidwall/btree"
	"os"
	"testing"
)

// saves the leaves to disk.
func TestTree_SaverSimple(t *testing.T) {
	tree := NewArtTree()
	// insert one key
	tree.Insert(Key("I'm Key"), ByteSliceValue("I'm Value"))
	//insert another key
	tree.Insert(Key("I'm Key2"), ByteSliceValue("I'm Value2"))

	//vv("as string tree = \n%v", tree.String())

	// search first
	value, _, found := tree.FindExact(Key("I'm Key"))
	assert.True(t, found)
	//vv("before serz, first search value = '%#v'", value)
	assert.Equal(t, ByteSliceValue("I'm Value"), value)

	// save
	path := "out.saver_test"
	fd, err := os.Create(path)
	panicOn(err)
	saver, err := tree.NewTreeSaver(fd)
	panicOn(err)
	err = saver.Save()
	panicOn(err)
	fd.Close()
	if want, got := 2, saver.NumLeafWrit(); want != got {
		t.Fatalf("bad num leaf writ: want %v, got %v", want, got)
	}

	// load
	fd2, err := os.Open(path)
	panicOn(err)
	defer fd2.Close()

	reader := bufio.NewReader(fd2)
	loader, err := NewTreeLoader(reader)
	panicOn(err)
	tree2, err := loader.Load()
	panicOn(err)

	if tree2 == nil {
		panic("Why is tree2 nil?")
	}
	// search it
	value, _, found = tree2.FindExact(Key("I'm Key"))
	assert.True(t, found)
	//vv("value = '%#v'", value)
	assert.Equal(t, ByteSliceValue("I'm Value"), value)

	// search it
	value, _, found = tree2.FindExact(Key("I'm Key2"))
	assert.True(t, found)
	assert.Equal(t, ByteSliceValue("I'm Value2"), value)
}

func TestDepthString_MediumDeepTree(t *testing.T) {
	tree := NewArtTree()
	paths := loadTestFile("assets/medium.txt")
	for k := range paths {
		tree.Insert(paths[k], paths[k])
	}

	//vv("as string tree = \n%v", tree.String())

}

func testSaveTree(path string, tree *Tree) {
	fd, err := os.Create(path)
	panicOn(err)
	defer fd.Close()
	saver, err := tree.NewTreeSaver(fd)
	panicOn(err)
	err = saver.Save()
	panicOn(err)
}

func testLoadTree(path string) (tree *Tree, err error) {
	fd2, err := os.Open(path)
	panicOn(err)
	defer fd2.Close()
	reader := bufio.NewReader(fd2)
	loader, err := NewTreeLoader(reader)
	panicOn(err)
	return loader.Load()
}

func TestTree_Saver_LinuxPaths(t *testing.T) {
	tree := NewArtTree()
	t0 := time.Now()
	paths := loadTestFile("assets/linux.txt")
	e0 := time.Since(t0)
	t1 := time.Now()
	for k := range paths {
		tree.Insert(paths[k], paths[k])
		//tree.Insert(paths[k], nil)
	}
	e1 := time.Since(t1)

	t2 := time.Now()
	path := "out.tree_linux_leaves"
	testSaveTree(path, tree)
	e2 := time.Since(t2)

	t3 := time.Now()
	tree2, err := testLoadTree(path)
	panicOn(err)
	e3 := time.Since(t3)
	_ = tree2

	// verify the read from disk
	t4 := time.Now()
	for k := range paths {
		key := paths[k]
		v, _, found := tree.FindExact(key)
		if !found {
			panic(fmt.Sprintf("missing key '%v'", string(key)))
		}
		if v == nil {
			panic(fmt.Sprintf("no value back from disk for key '%v'", string(key)))
		}
		if v != nil {
			gotVal := v.([]byte)
			if !bytes.Equal(gotVal, key) {
				panic(fmt.Sprintf("value from disk wrong; want '%v'; got '%v'",
					string(key), string(gotVal)))
			}
		}
	}
	e4 := time.Since(t4)

	fmt.Printf("time to load %v paths from disk: %v\n", len(paths), e0)
	fmt.Printf("time to insert paths into tree: %v\n", e1)
	fmt.Printf("time to save tree to disk: %v\n", e2)
	fmt.Printf("time to read disk tree into memory: %v\n", e3)
	fmt.Printf("time to search tree to verify all paths present: %v\n", e4)
}

// just a map[string][]byte for comparisoin.
func TestBasicMap_LinuxPaths(t *testing.T) {

	//time to load 93790 paths from disk: 27.915326ms
	//time to insert paths into map: 29.895015ms

	m := make(map[string][]byte)
	t0 := time.Now()
	paths := loadTestFile("assets/linux.txt")
	e0 := time.Since(t0)
	t1 := time.Now()
	for k := range paths {
		//tree.Insert(paths[k], paths[k])
		m[string(paths[k])] = paths[k]
	}
	e1 := time.Since(t1)

	fmt.Printf("time to load %v paths from disk: %v\n", len(paths), e0)
	fmt.Printf("time to insert paths into map: %v\n", e1)
}

// not finished, the bulk saver algorithm is just
// roughly sketched/drafter, and not bottom up tested!
func TestTree_Bulk2_Saver_LinuxPaths(t *testing.T) {

	return

	//tree := NewArtTree()
	t0 := time.Now()
	paths := loadTestFile3("assets/linux.txt")
	e0 := time.Since(t0)
	t1 := time.Now()

	bulk2(paths, 0)

	e1 := time.Since(t1)
	fmt.Printf("time to load %v paths from disk: %v\n", len(paths), e0)
	fmt.Printf("time to insert paths into tree: %v\n", e1)

}

const (
	maxPrefixLen int = 10
)

// This is an incomplete, untested implementation
// of the bulk-loading algorithm that the ART
// paper describes as a proposed "optimization".
// It remains to be seen if this really offers
// any benefits.
//
// invar: paths[i].key[depth] has to be valid,
// so len(paths[i].key) must be > depth for all i.
// hence when we recurse to depth+1, we cannot
// pass any len(paths[i].key) <= depth+1,
// aka      len(paths[i].key) < depth.
//
// PRE: paths[i][:depth] are all the same, for all i.
// that is, paths[i][:depth] = paths[j][:depth] for all i,j.
// This is their common prefix.
func bulk2(paths []*KVI, depth int) *bnode {

	if len(paths) == 0 {
		panic("cannot call without any paths")
	}
	if len(paths) == 1 {
		// leaf, by definition
		lf := NewLeaf(paths[0].Key, paths[0].Vidx, nil)
		return bnodeLeaf(lf)
	}

	var prefix [maxPrefixLen]byte
	copy(prefix[:], paths[0].Key[:depth])

	nused := 0
	var used [256]bool
	var radix [256][]*KVI
	//leaf := &Leaf{}

	for _, kv := range paths {
		if depth >= len(kv.Key) {
			// cannot recurse on this one,
			// no more bytes left
			panic("key too small to be recursed on") // hitting this
		} else {
			idx := kv.Key[depth]
			if !used[idx] {
				nused++
				used[idx] = true
			}
			radix[idx] = append(radix[idx], kv)
		}
	}

	//var node node

	switch {
	case nused < 5:
		n4 := &node4{
			lth: nused,
		}
		i := 0
		for j, inUse := range used {
			if inUse {
				n4.keys[i] = byte(j)
				n4.children[i] = bulk2(radix[j], depth+1)
				i++
			}
		}
		inn := &Inner{
			//Prefix:    prefix,
			//PrefixLen: depth,
			Node: n4,
		}
		inn.compressed = prefix[:]
		return bnodeInner(inn)
	case nused < 17:
		n16 := &node16{
			lth: nused,
		}
		i := 0
		for j, inUse := range used {
			if inUse {
				n16.keys[i] = byte(j)
				n16.children[i] = bulk2(radix[j], depth+1)
				i++
			}
		}
		//n16.compressed = prefix[:]
		inn := &Inner{
			//Prefix:    prefix,
			//PrefixLen: depth,
			compressed: prefix[:],
			Node:       n16,
		}
		return bnodeInner(inn)

	case nused < 49:
		n48 := &node48{
			lth: nused,
		}
		i := 0
		for j, inUse := range used {
			if inUse {
				n48.keys[i] = uint16(i)
				n48.children[i] = bulk2(radix[j], depth+1)
				i++
			}
		}
		//n48.compressed = prefix[:]
		inn := &Inner{
			//Prefix:    prefix,
			//PrefixLen: depth,
			compressed: prefix[:],
			Node:       n48,
		}
		return bnodeInner(inn)
	default:
		n256 := &node256{
			lth: nused,
		}
		i := 0
		for j, inUse := range used {
			if inUse {
				n256.children[i] = bulk2(radix[j], depth+1)
				i++
			}
		}
		//n256.compressed = prefix[:]
		inn := &Inner{
			compressed: prefix[:],
			//PrefixLen: depth,
			Node: n256,
		}
		return bnodeInner(inn)
	}
}

func loadTestFile3(path string) []*KVI {
	file, err := os.Open(path)
	if err != nil {
		panic("Couldn't open " + path)
	}
	defer file.Close()

	var words []*KVI
	reader := bufio.NewReader(file)
	for i := 0; ; i++ {
		if line, err := reader.ReadBytes(byte('\n')); err != nil {
			break
		} else {
			if len(line) > 0 {
				words = append(words, &KVI{Key: line[:len(line)-1], Vidx: i})
			}
		}
	}
	return words
}

type KVI struct {
	Key  []byte
	Vidx int
}
