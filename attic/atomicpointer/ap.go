package main

import (
	"fmt"
	"sync/atomic"
)

type n4 struct {
	key int
}

type n16 struct {
	key int
}

func (n *n16) Key() int {
	return n.key
}
func (n *n4) Key() int {
	return n.key
}

// type AtomicNode struct {
// 	//node atomic.Value // stores *NodeInterface
// 	node atomic.Pointer[NodeInterface]
// }

// func (a *AtomicNode) StoreInode(n NodeInterface) {
// 	a.node.StoreInode(&n)
// }

// func (a *AtomicNode) LoadInode() NodeInterface {
// 	ptr := a.node.LoadInode().(*NodeInterface)
// 	return *ptr
// }

func main() {

	an := &AtomicNode{}

	n4 := &n4{key: 4}
	n16 := &n16{key: 16}

	an.StoreInode(n4)
	got := an.LoadInode()
	fmt.Printf("stored n4, got back '%#v'\n", got)
	an.StoreInode(n16)
	got2 := an.LoadInode()
	fmt.Printf("stored n16, got2 back '%#v'\n", got2)

	an.StoreBytes([]byte("bytes 3"))
	got3 := an.LoadBytes()
	fmt.Printf("stored n4, got back '%#v'\n", string(got3))
	an.StoreBytes([]byte("bytes 4"))
	got4 := an.LoadBytes()
	fmt.Printf("stored n16, got2 back '%#v'\n", string(got4))

}

type NodeInterface interface {
	// Your methods
	Key() int
}

type AtomicNode struct {
	nodePtr  atomic.Pointer[NodeInterface]
	bytesPtr atomic.Pointer[[]byte]
}

func (a *AtomicNode) StoreInode(n NodeInterface) {
	a.nodePtr.Store(&n)
}

func (a *AtomicNode) LoadInode() NodeInterface {
	return *a.nodePtr.Load()
}

func (a *AtomicNode) StoreBytes(by []byte) {
	a.bytesPtr.Store(&by)
}

func (a *AtomicNode) LoadBytes() []byte {
	return *a.bytesPtr.Load()
}

/*
For atomic updates of multiple fields without using sync.Mutex, you need to use a pattern called "copy-on-write" or "immutable updates." This involves creating a new instance with the updated fields and atomically replacing the pointer to the old instance with a pointer to the new one.
Here's how you could implement this for your ART tree nodes:

// Define your node structure with the fields you need
type Node struct {
    Field1 interface{}
    Field2 interface{}
    // other fields
}

// AtomicNode holds a pointer that can be atomically updated
type AtomicNode struct {
    nodePtr atomic.Pointer[Node]
}

// Initialize the atomic node
func NewAtomicNode(field1, field2 interface{}) *AtomicNode {
    an := &AtomicNode{}
    an.nodePtr.Store(&Node{
        Field1: field1,
        Field2: field2,
    })
    return an
}

// Atomically load both fields
func (a *AtomicNode) Load() (field1, field2 interface{}) {
    node := a.nodePtr.Load()
    return node.Field1, node.Field2
}

// Atomically store both fields
func (a *AtomicNode) Store(field1, field2 interface{}) {
    // Create a new node with the updated fields
    newNode := &Node{
        Field1: field1,
        Field2: field2,
    }

    // Atomically replace the old node with the new one
    a.nodePtr.Store(newNode)
}

// CompareAndSwap allows for atomic conditional updates
func (a *AtomicNode) CompareAndSwap(oldField1, oldField2, newField1, newField2 interface{}) bool {
    // Load the current node
    oldNode := a.nodePtr.Load()

    // Check if the current values match the expected values
    if oldNode.Field1 != oldField1 || oldNode.Field2 != oldField2 {
        return false
    }

    // Create a new node with updated values
    newNode := &Node{
        Field1: newField1,
        Field2: newField2,
    }

    // Try to swap - returns true if successful
    return a.nodePtr.CompareAndSwap(oldNode, newNode)
}
*/

/*
The key aspects that make this work:

The atomic.Int32 provides an atomic way to switch which buffer is considered "current"
Readers only access the current buffer and don't need locks
Writers only modify the inactive buffer, then atomically make it active
The writeLock ensures only one writer can update the inactive buffer at a time

This approach works well when:

Readers should never be blocked
You need to update multiple fields atomically
The cost of maintaining two copies is acceptable

The atomic swap itself is just a single atomic store operation to the current counter, which makes the previously inactive buffer become the active one that all readers will now use.



type DoubleBufferedNode struct {
	current   atomic.Int32 // Indicates which buffer is currently active (0 or 1)
	buffers   [2]*Node
	writeLock sync.Mutex // Protects updates to the inactive buffer
}

// Create a new double-buffered node
func NewDoubleBufferedNode() *DoubleBufferedNode {
	return &DoubleBufferedNode{
		buffers: [2]*Node{
			&Node{}, // Initialize both buffers
			&Node{},
		},
	}
	// current defaults to 0
}

// Get the currently active buffer for reading (lock-free)
func (db *DoubleBufferedNode) GetCurrent() *Node {
	currentIdx := db.current.Load()
	return db.buffers[currentIdx]
}

// Update both fields atomically
func (db *DoubleBufferedNode) Update(field1, field2 interface{}) {
	// Lock to prevent concurrent modifications to the inactive buffer
	db.writeLock.Lock()
	defer db.writeLock.Unlock()

	// Determine which buffer is currently inactive
	currentIdx := db.current.Load()
	inactiveIdx := 1 - currentIdx

	// Update the inactive buffer
	inactiveBuffer := db.buffers[inactiveIdx]

	// Copy any necessary data from the current buffer
	// (if there are fields we're not updating)
	currentBuffer := db.buffers[currentIdx]
	// ... copy any unchanged fields ...

	// Set the new values
	inactiveBuffer.Field1 = field1
	inactiveBuffer.Field2 = field2

	// Atomically switch to the newly updated buffer
	db.current.Store(inactiveIdx)
}
*/
