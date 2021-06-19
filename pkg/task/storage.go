package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"
	"github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	// database key prefixes
	prefixScheduled  = "queue"
	prefixProcessing = "current"
	prefixComplete   = "archive"

	ErrNotFound = errors.New("task not found")
)

// Tasks stored in leveldb
type Storage struct {
	db *leveldb.DB
}

// derive the key from the database prefix and the ID of the task we are searching for.
// In order to do time-based range searches and searches for tasks in a particular phase of execution,
// keys are stored under a prefix which represents the state of the task and a timestamp.
//
// Tasks are using a time-based identifier (using the library xid) to identify each task. These ids
// are also lexicographically sortable. However, for the key inside LevelDB we're just using a key
// composed by the prefix + the timestamp + the xid.
//
// This way we can easily range over periods of time and, at the same time, get specific tasks from
// the storage.
func taskKey(prefix string, id string) ([]byte, error) {
	u, err := xid.FromString(id)
	if err != nil {
		return nil, errors.New("task key must be a xid id")
	}

	tskey := strconv.FormatInt(u.Time().Unix(), 10) + "_" + u.String()
	return []byte(strings.Join([]string{prefix, tskey}, ":")), nil
}

func (s *Storage) get(prefix string, id string) (tsk *Task, err error) {
	tsk = &Task{}
	key, err := taskKey(prefix, id)
	if err != nil {
		return nil, err
	}
	val, err := s.db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(val, tsk)
	if err != nil {
		return nil, err
	}
	return tsk, nil
}

func (s *Storage) put(prefix string, tsk *Task) error {
	val, err := json.Marshal(tsk)
	if err != nil {
		return err
	}
	key, err := taskKey(prefix, tsk.ID)
	if err != nil {
		return err
	}
	return s.db.Put(key, val, &opt.WriteOptions{
		Sync: true,
	})
}

func (s *Storage) Delete(id string) error {
	tsk, err := s.get(prefixComplete, id)
	if err == nil {
		return s.delete(prefixComplete, tsk)
	}
	if err != ErrNotFound {
		return err
	}
	tsk, err = s.get(prefixProcessing, id)
	if err == nil {
		return s.delete(prefixProcessing, tsk)
	}
	if err != ErrNotFound {
		return err
	}
	tsk, err = s.get(prefixScheduled, id)
	if err == nil {
		return s.delete(prefixScheduled, tsk)
	}
	if err != ErrNotFound {
		return err
	}
	return errors.New("should have found task")
}

func (s *Storage) delete(prefix string, tsk *Task) error {
	key, err := taskKey(prefix, tsk.ID)
	if err != nil {
		return err
	}
	return s.db.Delete(key, &opt.WriteOptions{
		Sync: true,
	})
}

func (s *Storage) Get(id string) (*Task, error) {
	tsk, err := s.get(prefixComplete, id)
	if err == nil {
		return tsk, nil
	}
	if err != ErrNotFound {
		return nil, err
	}
	tsk, err = s.get(prefixProcessing, id)
	if err == nil {
		return tsk, nil
	}
	if err != ErrNotFound {
		return nil, err
	}
	return s.get(prefixScheduled, id)
}

func (s *Storage) PersistProcessing(tsk *Task) error {
	return s.put(prefixProcessing, tsk)
}

func (s *Storage) PersistScheduled(tsk *Task) error {
	return s.put(prefixScheduled, tsk)
}

func (s *Storage) ProcessTask(tsk *Task) error {
	return s.changePrefix(prefixProcessing, prefixScheduled, tsk.ID)
}

func (s *Storage) ArchiveTask(tsk *Task) error {
	return s.changePrefix(prefixComplete, prefixProcessing, tsk.ID)
}

// Change the prefix of a task
func (s *Storage) changePrefix(dst string, src string, id string) error {
	oldkey, err := taskKey(src, id)
	if err != nil {
		return err
	}
	newkey, err := taskKey(dst, id)
	if err != nil {
		return err
	}
	trans, err := s.db.OpenTransaction()
	if err != nil {
		return err
	}
	val, err := trans.Get(oldkey, nil)
	if err != nil {
		trans.Discard()
		return err
	}
	err = trans.Put(newkey, val, &opt.WriteOptions{Sync: true})
	if err != nil {
		trans.Discard()
		return err
	}
	err = trans.Delete(oldkey, &opt.WriteOptions{Sync: true})
	if err != nil {
		trans.Discard()
		return err
	}
	return trans.Commit()
}

func (s *Storage) Filter(state State, start time.Time, end time.Time) (tasks []*Task, err error) {
	var prefix string

	switch state {
	case StateScheduled:
		prefix = prefixScheduled
	case StateProcessing:
		prefix = prefixProcessing
	case StateComplete:
		prefix = prefixComplete
	}

	return s.rangeIter(prefix, start, end)
}

// rangeIter returns []*Task with all tasks between the given time ranges.
func (s *Storage) rangeIter(prefix string, start time.Time, end time.Time) (tasks []*Task, err error) {
	rng := util.Range{
		Start: []byte(strings.Join([]string{
			prefix,
			strconv.FormatInt(start.Unix(), 10),
		}, ":")),
		Limit: []byte(strings.Join([]string{
			prefix,
			strconv.FormatInt(end.Unix(), 10),
		}, ":")),
	}

	tasks = make([]*Task, 0)

	iter := s.db.NewIterator(&rng, nil)
	defer iter.Release()

	for iter.Next() {
		tsk := &Task{}

		err := json.Unmarshal(iter.Value(), tsk)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, tsk)
	}
	return tasks, nil
}

func NewMemoryTaskStorage() (*Storage, error) {
	inmem := storage.NewMemStorage()
	db, err := leveldb.Open(inmem, nil)
	if err != nil {
		return nil, err
	}
	return &Storage{db}, nil
}

func NewTaskStorage(path string) (*Storage, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, fmt.Errorf("error while opening storage: %v", err)
	}
	return &Storage{db}, nil
}
