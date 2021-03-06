package queue

import (
	"encoding/binary"
	"errors"
	"os"
	"regexp"
	"sync"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// Queue represents a persistent FIFO structure
// that stores the data in leveldb
type Queue struct {
	sync.RWMutex
	Name     string
	DataDir  string
	Stats    *Stats
	head     uint64
	tail     uint64
	db       *leveldb.DB
	isOpened bool
}

//Stats contains queue level stats
type Stats struct {
	OpenTransactions int64
}

// Item represents a queue item
type Item struct {
	Key   []byte
	Value []byte
	Size  int32
}

// Open creates a queue and opens underlying leveldb database
func Open(name string, dataDir string) (*Queue, error) {
	q := &Queue{
		Name:     name,
		DataDir:  dataDir,
		Stats:    &Stats{0},
		db:       &leveldb.DB{},
		head:     0,
		tail:     0,
		isOpened: false,
	}
	return q, q.open()
}

// Close leveldb database
func (q *Queue) Close() {
	if q.isOpened {
		q.db.Close()
	}
	q.isOpened = false
}

// Drop closes and deletes leveldb database
func (q *Queue) Drop() {
	q.Close()
	os.RemoveAll(q.Path())
}

// Head returns current head offset of the queue
func (q *Queue) Head() uint64 { return q.head }

// Tail returns current tail offset of the queue
func (q *Queue) Tail() uint64 { return q.tail }

// Length returns current length of the queue
func (q *Queue) Length() uint64 {
	q.RLock()
	defer q.RUnlock()
	return q.length()
}

// Peek returns next queue item without removing it from the queue
func (q *Queue) Peek() (*Item, error) {
	q.RLock()
	defer q.RUnlock()

	return q.peek()
}

// Enqueue adds new value to the queue
func (q *Queue) Enqueue(value []byte) error {
	q.Lock()
	defer q.Unlock()

	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, q.tail+1)
	err := q.db.Put(key, value, nil)
	if err == nil {
		q.tail++
	}
	return err
}

// Dequeue returns next queue item and removes it from the queue
func (q *Queue) Dequeue() (*Item, error) {
	q.Lock()
	defer q.Unlock()

	item, err := q.peek()
	if err != nil {
		return item, err
	}

	err = q.db.Delete(item.Key, nil)
	if err == nil {
		q.head++
	}
	return item, err
}

// Prepend adds new queue intem in from of the queue
func (q *Queue) Prepend(item *Item) error {
	q.Lock()
	defer q.Unlock()
	if q.head < 1 {
		return errors.New("Queue head can not be less then zero")
	}
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, q.head)
	err := q.db.Put(key, item.Value, nil)
	if err == nil {
		q.head--
	}
	return err
}

// AddOpenTransactions increments OpenTransactions stats item
func (q *Queue) AddOpenTransactions(value int64) {
	atomic.AddInt64(&q.Stats.OpenTransactions, value)
}

// Path returns leveldb database file path
func (q *Queue) Path() string {
	return q.DataDir + "/" + q.Name
}

func (q *Queue) open() error {
	q.Lock()
	defer q.Unlock()
	if regexp.MustCompile(`[^a-zA-Z0-9_]+`).MatchString(q.Name) {
		return errors.New("Queue name is not alphanumeric")
	}

	if len(q.Name) > 100 {
		return errors.New("Queue name is too long")
	}

	o := opt.Options{
		BlockCacher:       opt.NoCacher,
		DisableBlockCache: true,
	}

	var err error
	q.db, err = leveldb.OpenFile(q.Path(), &o)
	if err != nil {
		return err
	}
	q.isOpened = true
	return q.initialize()
}

func (q *Queue) length() uint64 {
	return q.tail - q.head
}

func (q *Queue) peek() (*Item, error) {
	if q.length() < 1 {
		return &Item{nil, nil, 0}, errors.New("Queue is empty")
	}

	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, q.head+1)
	value, err := q.db.Get(key, nil)
	item := &Item{key, value, int32(len(value))}
	return item, err
}

func (q *Queue) initialize() error {
	iter := q.db.NewIterator(nil, nil)
	defer iter.Release()

	if iter.First() {
		q.head = binary.BigEndian.Uint64(iter.Key()) - 1
	}

	if iter.Last() {
		q.tail = binary.BigEndian.Uint64(iter.Key())
	}

	return iter.Error()
}
