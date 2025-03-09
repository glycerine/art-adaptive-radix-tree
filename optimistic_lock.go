package art

// at very top was go build !race

// https://github.com/dshulyak/art

import (
	"runtime"
	//"sync"
	"sync/atomic"
)

const underRaceDetector = false

// olock is a implemention of an Optimistic Lock.
// As descibed in https://15721.courses.cs.cmu.edu/spring2017/papers/08-oltpindexes2/leis-damon2016.pd// Appendix A: Implementation of Optimistic Locks
//
// "The ART of Practical Synchronization"
// by Viktor Leis, Florian Scheibner, Alfons Kemper, Thomas Neumann
//
// The two least significant bits indicate if the node is obsolete
// or if the node is locked, respectively.  The remaining bits
// store the update counter.
//
// Zero value is unlocked.
//
// Deeper background, from the paper:
/*
"Lock coupling is simple and seems to allow for
a high degree of parallelism. However, as we
show in Section 5, it performs very badly on
modern multi-core CPUs even if the locks do not
logically conflict at all, for example in
read-only workloads.

"The reason is that concurrent locking of tree structures
causes many unnecessary cache misses: Each time a core
acquires a read lock for a node (by writing
to that node), all copies of that
cache line are invalidated in the caches of
all other cores. Threads, in effect, “fight” for
exclusive ownership of the cache line holding
the lock. The root node and other nodes
close to it become contention points. Therefore,
other synchronization mechanisms are necessary
to fully utilize modern multi-core CPUs.

"Optimistic Lock Coupling is similar to "normal"
lock coupling, but offers dramatically better
scalability. Instead of preventing concurrent
modifications of nodes (as locks do), the basic
idea is to optimistically assume that there
will be no concurrent modification.

"Modifications are detected after the fact
using version counters, and the operation is
restarted if necessary. From a performance
standpoint, this optimism makes a huge difference,
because it dramatically reduces the number
of writes to shared memory locations."
*/
type olock struct {
	//_       sync.Mutex // for compiler warning if Mutex is copied after first use
	version uint64
}

// RLock waits for node to be unlocked and returns current version, possibly obsolete.
// If version is obsolete user must discard used object and restart execution.
// Read lock is a current version value, if this value gets outdated at the time of RUnlock
// read will need to be restarted.
func (ol *olock) RLock() (vers uint64, obsolete bool) {
	version := ol.waitUnlocked()
	return version, isObsolete(version)
}

// RUnlock compares read lock with current value of the olock, in case if
// value got changed - RUnlock will return true (and
// call locked.Unlock(), if locked is provided).
func (ol *olock) RUnlock(version uint64, locked *olock) (obsolete bool) {
	if atomic.LoadUint64(&ol.version) != version {
		if locked != nil {
			locked.Unlock()
		}
		return true
	}
	return false
}

// Upgrade current lock to write lock, in case of failure to
// update locked lock will be unlocked.
//
// Returns true if the version changes, which means
// you need to restart the operation.
func (ol *olock) Upgrade(version uint64, locked *olock) (mustRestart bool) {

	if !atomic.CompareAndSwapUint64(&ol.version, version, setLockedBit(version)) {
		if locked != nil {
			locked.Unlock()
		}
		return true
	}
	return false
}

// Check returns true if version has changed.
func (ol *olock) Check(version uint64) (obsolete bool) {
	return !(atomic.LoadUint64(&ol.version) == version)
}

// aka WriteLock (spinlock).
func (ol *olock) Lock() {
	var (
		version  uint64
		obsolete = true
	)
	for obsolete {
		version, obsolete = ol.RLock()
		if obsolete {
			continue
		}
		// there we no other writers when
		// we got our version. See if we can be
		// the next writer (we are if we get
		// back false for obsolete.)
		// No new readers or writers will be
		// allowed in. Prior readers? They
		// are already in, but will notice
		// that they need to begin again
		// on RUnlock() returning obsolete.
		//
		// What then prevents a reader from then
		// reading a nil pointer that the
		// writer writes?
		obsolete = ol.Upgrade(version, nil)
	}
}

func (ol *olock) Unlock() {
	// "reset locked bit and overflow into version".
	atomic.AddUint64(&ol.version, 2)
}

func (ol *olock) UnlockObsolete() {
	// "set obsolete, reset locked, overflow into version"
	atomic.AddUint64(&ol.version, 3)
}

func (ol *olock) waitUnlocked() uint64 {
	for {
		version := atomic.LoadUint64(&ol.version)
		if version&2 != 2 {
			return version
		}

		// runtime docs: "Gosched yields the processor,
		// allowing other goroutines to run.
		// It does not suspend the current goroutine,
		// so execution resumes automatically."
		runtime.Gosched()
	}
}

func isObsolete(version uint64) bool {
	return (version & 1) == 1
}

func setLockedBit(version uint64) uint64 {
	return version + 2
}
