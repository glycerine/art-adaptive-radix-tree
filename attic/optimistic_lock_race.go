//go:build race
// +build race

package art

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
)

const underRaceDetector = true

// our simulated olocks (to make the
// race detector happy) have only 3 states:
// unlocked, read-locked, and write-locked.
type simu int

const (
	simu_Unlocked    simu = 0
	simu_ReadLocked  simu = 1
	simu_WriteLocked simu = 2
)

func (s simu) String() string {
	switch s {
	case simu_Unlocked:
		return "simu_Unlocked"
	case simu_ReadLocked:
		return "simu_ReadLocked"
	case simu_WriteLocked:
		return "simu_WriteLocked"
	}
	panic("unknown simu")
}

// olock implements pessimistic locking. We need
// this alternative version because the race
// detector won't be able to recognize the
// correctness of the optimistic locking
// and will report races if tests are executed
// with -race flag
//
// Our approach is simply to
// simulate a mutex with a condition
// variable so we can simulate the special things
// the optimistic locking has:
//
// a) multiple re-entry being idempotent;
// b) being able to upgrade a from read
// to a write lock
// c) others things to match the optimistic locks...
//
// Basically the condition variable lets us simulate
// the hand-over-hand locking but with pessimistic
// locking (and thus way less concurrency) for
// the race detector.
//
// It is massively slower than the optimistic locking.
type olock struct {
	mu    sync.Mutex
	state simu

	cv      *sync.Cond
	version uint64

	writer int // just one exclusively

	// just one reader, exclusively.
	// If we allow multiple readers, then they
	// prevent each other from upgrading to
	// an exclusive write lock, which is actually
	// the most common typical path. Hence, in
	// order to keep the application code the same
	// with and without -race, we must
	// only allow one reader at a time here.
	reader int

	// any other readers/writings in fifo order,
	// to guarantee progress.
	waiters []int
}

func (ol *olock) init() {
	if ol.cv != nil {
		return // init already done
	}
	ol.cv = sync.NewCond(&ol.mu)
}

func (ol *olock) myTurnToRead() bool {
	gn := GoroNumber()
	if ol.writer == 0 && ol.reader == 0 && ol.waiters[0] == gn {
		ol.waiters = ol.waiters[1:]
		ol.reader = gn
		return true
	}
	// else wait
	return false
}

// never seen! our read->write upgrade path
// short circuits all ecounters with myTurnToWrite
// in the ConcurrentInsert tests.
func (ol *olock) myTurnToWrite() bool {
	gn := GoroNumber()
	if ol.writer == 0 && ol.reader == 0 && ol.waiters[0] == gn {
		ol.waiters = ol.waiters[1:]
		ol.writer = gn
		return true
	}
	return false
}

func (ol *olock) writing() {
	ol.doneWaiting()
	gn := GoroNumber()
	ol.writer = gn
	ol.reader = 0
}

func (ol *olock) amWriting() bool {
	return ol.writer == GoroNumber()
}

func (ol *olock) doneWriting() {
	ol.writer = 0
}

func (ol *olock) waiting() {
	gn := GoroNumber()
	for _, r := range ol.waiters {
		if r == gn {
			return
		}
	}
	ol.waiters = append(ol.waiters, gn)
}

func (ol *olock) doneWaiting() {
	gn := GoroNumber()
	for i, r := range ol.waiters {
		if r == gn {
			ol.waiters = append(ol.waiters[:i], ol.waiters[i+1:]...)
			return
		}
	}
}

func (ol *olock) reading() {
	ol.doneWaiting()
	ol.reader = GoroNumber()
	ol.writer = 0
}

func (ol *olock) amReading() bool {
	return ol.reader == GoroNumber()
}

func (ol *olock) doneReading() {
	ol.reader = 0
}

func (ol *olock) RLock() (vers uint64, obsolete bool) {
	//vv("about to RLock() mu.Lock")
	ol.mu.Lock() // hung here on art2_test iteration, under race.
	//vv("past RLock() mu.Lock")
	if ol.cv == nil {
		ol.init()
	}
	switch ol.state {
	case simu_WriteLocked:

		// wait for it to become unlocked
		ol.waiting()
		for {
			if ol.state == simu_Unlocked && ol.myTurnToRead() {
				break
			}
			ol.cv.Wait()
		}
		ol.state = simu_ReadLocked

	case simu_ReadLocked:
		// only one reader at a time.
		if ol.amReading() {
			// it is already us, continue
		} else {
			ol.waiting()
			for {
				if ol.state == simu_Unlocked && ol.myTurnToRead() {
					break
				}
				ol.cv.Wait()
			}
			ol.state = simu_ReadLocked
		}
	case simu_Unlocked:
		ol.state = simu_ReadLocked
		ol.reading()
	}
	vers = ol.version
	ol.mu.Unlock()
	return vers, false
}

// RUnlock compares read lock with current value of the olock, in case if
// value got changed - RUnlock will return true.
func (ol *olock) RUnlock(version uint64, locked *olock) bool {
	ol.mu.Lock()
	if ol.cv == nil {
		ol.init()
	}
	defer ol.cv.Broadcast()

	if !ol.amReading() {
		// happens, have to allow it.
		ol.mu.Unlock()
		return false
	}
	// INVAR: we are reading.
	ol.doneReading()
	switch ol.state {
	case simu_WriteLocked:
		// writer-writer conflict. not seen, good.
		panic("how to deal with: an RUnlock() call when WriteLocked.")
	case simu_ReadLocked:
		ol.state = simu_Unlocked
	case simu_Unlocked:
		// no change
	}
	ol.mu.Unlock()
	return false
}

// Upgrade current lock to write lock, in case of failure
// to update locked lock will be unlocked.
func (ol *olock) Upgrade(version uint64, locked *olock) (mustRestart bool) {
	ol.mu.Lock()
	if ol.cv == nil {
		ol.init()
	}

	switch ol.state {
	case simu_WriteLocked:
		// conflicting writers?

		if ol.amWriting() {
			// fine, its still us.
			ol.mu.Unlock()
			return false
		}

		// queue up
		ol.waiting()

		// wait for other writers/readers to finish.
		for {
			if ol.state == simu_Unlocked && ol.myTurnToWrite() {
				break
			}
			ol.cv.Broadcast()
			ol.cv.Wait()
		}
		ol.state = simu_WriteLocked

	case simu_ReadLocked:

		if ol.amReading() {
			// most common path:
			ol.writing()
			ol.state = simu_WriteLocked
			ol.version += 2
			ol.mu.Unlock()
			return false
		}

		// wait for other readers/writers to finish.
		for {
			if ol.state == simu_Unlocked && ol.myTurnToWrite() {
				break
			}
			ol.cv.Wait()
		}
		ol.state = simu_WriteLocked
		ol.version += 2

	case simu_Unlocked:
		ol.state = simu_WriteLocked
		ol.writing()
		ol.version += 2
	}
	ol.mu.Unlock()
	return false
}

// Check returns true if version has changed.
func (ol *olock) Check(version uint64) (obsolete bool) {
	return false
}

func (ol *olock) Lock() {
	ol.mu.Lock()
	if ol.cv == nil {
		ol.init()
	}

	switch ol.state {
	case simu_WriteLocked:
		if ol.amWriting() {
			// fine, its still us.
			ol.mu.Unlock()
			return
		}
		// queue up
		ol.waiting()

		// wait for other writers/readers to finish.
		for {
			if ol.state == simu_Unlocked && ol.myTurnToWrite() {
				break
			}
			ol.cv.Wait()
		}
		ol.state = simu_WriteLocked

	case simu_ReadLocked:

		if ol.amReading() {
			// fine, we are upgrading to writer.
			ol.state = simu_WriteLocked
			ol.writing()
			ol.mu.Unlock()
			return
		}
		// queue up
		ol.waiting()

		// wait for other writers/readers to finish.
		for {
			if ol.state == simu_Unlocked && ol.myTurnToWrite() {
				break
			}
			ol.cv.Broadcast()
			ol.cv.Wait()
		}
		ol.state = simu_WriteLocked

	case simu_Unlocked:
		ol.writing()
		ol.state = simu_WriteLocked
	}

	ol.mu.Unlock()
}

func (ol *olock) Unlock() {
	ol.mu.Lock()
	if ol.cv == nil {
		ol.init()
	}
	defer ol.cv.Broadcast()

	if !ol.amWriting() {
		panic(fmt.Sprintf("non writer calls unlock?!? %v", GoroNumber()))
	}
	ol.doneWriting()

	switch ol.state {
	case simu_WriteLocked:
		ol.version += 2
		ol.state = simu_Unlocked
	case simu_ReadLocked:
		panic("writer unlock happens when read locked??")
	case simu_Unlocked:
		// idempotent
	}
	ol.mu.Unlock()
}

func (ol *olock) UnlockObsolete() {
	ol.mu.Lock()
	if ol.cv == nil {
		ol.init()
	}
	defer ol.cv.Broadcast()

	if !ol.amWriting() {
		panic(fmt.Sprintf("non writer calls UnlockObsolete?!? %v", GoroNumber()))
	}
	ol.doneWriting()

	ol.state = simu_Unlocked
	ol.mu.Unlock()
}

// GoroNumber returns the calling goroutine's number.
func GoroNumber() int {
	buf := make([]byte, 48)
	nw := runtime.Stack(buf, false) // false => just us, no other goro.
	buf = buf[:nw]

	// prefix "goroutine " is len 10.
	i := 10
	for buf[i] != ' ' && i < 30 {
		i++
	}
	n, err := strconv.Atoi(string(buf[10:i]))
	panicOn(err)
	return n
}
