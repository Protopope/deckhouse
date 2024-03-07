package cache

import (
	"errors"
	"io"
	"os"
	"sync"
)

var ErrNotFound = errors.New("not found")

type Cache interface {
	Get(digest string) (int64, io.ReadCloser, error)
	Set(digest string, size int64, reader io.Reader) error
}

type FileSystem struct {
	mutex sync.RWMutex
	dir   string
}

func NewFileSystemCache(dir string) (*FileSystem, error) {
	err := os.MkdirAll(dir, 0600)
	if err != nil {
		return nil, err
	}

	return &FileSystem{dir: dir}, nil
}

func (c *FileSystem) Get(digest string) (int64, io.ReadCloser, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	file, err := os.Open(c.dir + "/" + digest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil, ErrNotFound
		}

		return 0, nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		return 0, nil, err
	}

	return stat.Size(), file, nil
}

func (c *FileSystem) Set(digest string, size int64, reader io.Reader) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	file, err := os.OpenFile(c.dir+"/"+digest, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	if stat.Size() == size {
		return nil
	}

	_, err = io.Copy(file, reader)
	if err != nil {
		return err
	}

	return nil
}
