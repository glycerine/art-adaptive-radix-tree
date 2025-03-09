package art

import (
	"bytes"
	"fmt"
	"github.com/dshulyak/art"
	"github.com/stretchr/testify/assert"
	tbtree "github.com/tidwall/btree"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestArtTest_Insert(t *testing.T) {
	tree := NewArtTree()
	N := 1_000_000
	if underRaceDetector {
		N = 100
	}
	for i := 0; i < N; i++ {
		tree.Insert(Key(fmt.Sprintf("sharedNode::%d", i)), i)
	}
}

func TestTree_ConcurrentInsert1(t *testing.T) {
	t.Parallel()
	// set up
	//N := 1_000_000
	N := 1000 // 6 seconds
	tree := NewArtTree()
	wg := sync.WaitGroup{}
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			tree.Insert(Key(fmt.Sprintf("sharedNode::%d", i)), i)
			wg.Done()
		}(i)
	}
	wg.Wait()
	for i := 0; i < N; i++ {
		// if tree is our "haystack"...
		needle := fmt.Sprintf("sharedNode::%d", i)
		value, found := tree.FindExact(Key(needle))
		//assert.True(t, found) // red test here! on new one branch
		if !found {
			panic(fmt.Sprintf("could not find needle '%v'", needle))
		}
		//assert.Equal(t, i, value)
		got := value.(int)
		if i != got {
			panic(fmt.Sprintf("found needle '%v' but value off '%v' not '%v'", needle, got, i))
		}
	}
}

type orig struct {
	key     string
	i       int
	updated atomic.Bool
	back    atomic.Bool
}

func TestTree_ConcurrentInsert2(t *testing.T) {
	t.Parallel()
	// set up
	//N := 1_000_000
	N := 1000 // 6 seconds, okay under -race
	//N := 20 // was enough to locate the issue that needed clone.
	tree := NewArtTree()

	intent := make(chan *orig, N)
	inserted := make(chan *orig, N)

	show := func(title string, c chan *orig) {
		n := len(c)
		for i := range n {
			x := <-c
			fmt.Printf("%v[%03d]: %v updt: %v back:%v\n", title, x.i, string(x.key), x.updated.Load(), x.back.Load())
			if i%10 == 0 {
				fmt.Printf("\n")
			}
		}
	}
	_ = show

	//mu := sync.RWMutex{}
	//wg := sync.WaitGroup{}

	for i := 0; i < N; i++ {
		//wg.Add(1)
		go func(i int) {
			//rng := rand.New(rand.NewSource(time.Now().UnixNano()))
			//k := randomKey(rng)
			ks := randomID(20)
			k := []byte(ks)
			cp1 := append([]byte{}, k...)
			cp2 := append([]byte{}, k...)
			o := &orig{key: string(cp1), i: i}
			o2 := &orig{key: string(cp2), i: i}
			intent <- o
			//vv("inserting on i = %v; len(intent) = %v; len(inserted) = %v; key k = '%v'", i, len(intent), len(inserted), ks)
			updated := tree.Insert(k, k)
			if updated {
				panic(fmt.Sprintf("updated was true!?!?! key='%v'", ks)) // hit this under race (and not)! -- without clone in insert(). green with clone--but it is too slow.
			}
			o.updated.Store(updated)
			o.back.Store(true)
			//mu.Lock()
			//vv("inserted[%03d] '%v'", i, ks)
			inserted <- o2
			//mu.Unlock()
			//wg.Done()
		}(i)
	}
	//wg.Wait()

	for i := 0; i < N; i++ {
		ins := <-inserted
		value, found := tree.FindExact([]byte(ins.key))
		if !found {
			show("intent", intent)
			show("ins", inserted)
			fmt.Printf("i=%v key:'%v' not found!\n", ins.i, ins.key)

			if tree.root != nil {
				fmt.Printf("tree = %v\n", tree.String())
			} else {
				fmt.Printf("tree root was nil!\n")
			}
			os.Exit(1)
		}
		assert.True(t, found)
		assert.Equal(t, []byte(ins.key), value)
	}
}

func BenchmarkArtConcurrentInsert(b *testing.B) {
	value := newValue(123)
	l := NewArtTree()
	b.ResetTimer()
	//var count int
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		var rkey [8]byte

		for pb.Next() {
			rk := randomKey(rng, rkey[:])
			l.Insert(rk, value)
		}
	})
}

func BenchmarkAnotherArtConcurrentInsert(b *testing.B) {
	value := newValue(123)
	l := art.Tree{} // other package, "github.com/dshulyak/art"
	b.ResetTimer()
	//var count int
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		var rkey [8]byte
		for pb.Next() {
			rk := randomKey(rng, rkey[:])
			l.Insert(rk, value)
		}
	})
}

//func BenchmarkConcurrentInsert(b *testing.B) {
//	value := newValue(123)
//	l := NewArtTree()
//	b.ResetTimer()
//	//var count int
//	b.RunParallel(func(pb *testing.PB) {
//		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
//		for pb.Next() {
//			l.Insert(randomKey(rng), value)
//		}
//	})
//}

func BenchmarkBtreeConcurrentInsert(b *testing.B) {
	l := tbtree.NewGenericOptions[[]byte](func(a, b []byte) bool {
		return bytes.Compare(a, b) < 0
	}, tbtree.Options{NoLocks: false})
	b.ResetTimer()
	//var count int
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		var rkey [8]byte
		for pb.Next() {
			rk := randomKey(rng, rkey[:])
			l.Set(rk)
		}
	})
}
