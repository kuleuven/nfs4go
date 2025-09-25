package worker

import (
	"github.com/kuleuven/nfs4go/clients"
	"github.com/kuleuven/vfs"
)

type File struct {
	File        vfs.WriterAtReaderAt
	Handle      []byte
	Client      *clients.Client
	ClientSeqID uint32
}

func (w *Worker) AddFile(file *File) uint64 {
	w.Lock()
	defer w.Unlock()

	index := randUint64()

	for _, ok := w.Files[index]; ok; _, ok = w.Files[index] {
		index = randUint64()
	}

	w.Files[index] = file

	file.Client.Add(1)

	return index
}

func (w *Worker) GetFile(index uint64) (*File, bool) {
	w.Lock()
	defer w.Unlock()

	f, ok := w.Files[index]

	return f, ok
}

func (w *Worker) GetFileByClientSeqID(client *clients.Client, clientSeqID uint32) (uint64, bool) {
	w.Lock()
	defer w.Unlock()

	for index, f := range w.Files {
		if f.ClientSeqID == clientSeqID && f.Client == client {
			return index, true
		}
	}

	return 0, false
}

func (w *Worker) RemoveFile(index uint64) (*File, bool) {
	w.Lock()
	defer w.Unlock()

	f, ok := w.Files[index]
	if !ok {
		return nil, false
	}

	delete(w.Files, index)

	w.ClosedFiles[index] = struct{}{}

	f.Client.Done()

	return f, true
}

func (w *Worker) IsRemovedFile(index uint64) bool {
	w.Lock()
	defer w.Unlock()

	_, ok := w.ClosedFiles[index]

	return ok
}
