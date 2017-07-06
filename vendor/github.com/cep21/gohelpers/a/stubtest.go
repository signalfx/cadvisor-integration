package a

import (
	"sync"
	"os"
	"encoding/json"
	"bufio"
	"io"
	"io/ioutil"
)

// ---------------- file.WriteString(str)

type FileWriteStringObj struct {
	mutex sync.Mutex
	toCall func(f *os.File, str string) (int, error)
}

func (stub *FileWriteStringObj) UseFunction(toCall func(f *os.File, str string) (int, error)) {
	stub.mutex.Lock()
	stub.toCall = toCall
}

func (stub *FileWriteStringObj) Execute(f *os.File, str string) (int, error) {
	if stub.toCall == nil {
		stub.mutex.Lock()
		defer stub.mutex.Unlock()
		return f.WriteString(str)
	}
	return stub.toCall(f, str)
}

func (stub *FileWriteStringObj) Reset() {
	stub.toCall = nil
	stub.mutex.Unlock()
}

// -------------------- json.Unmarshal
type JsonUnmarshalObj struct {
	mutex sync.Mutex
	toCall func(data []byte, v interface{}) (error)
}

func (stub *JsonUnmarshalObj) UseFunction(toCall func(data []byte, v interface{}) (error)) {
	stub.mutex.Lock()
	stub.toCall = toCall
}

func (stub *JsonUnmarshalObj) Execute(data []byte, v interface{})  error {
	if stub.toCall == nil {
		stub.mutex.Lock()
		defer stub.mutex.Unlock()
		return json.Unmarshal(data ,v)
	}
	return stub.toCall(data, v)
}

func (stub *JsonUnmarshalObj) Reset() {
	stub.toCall = nil
	stub.mutex.Unlock()
}

// -------------------- Reader.ReadBytes
type ReaderReadBytesObj struct {
	changedCallMutex sync.Mutex

	toCallReadMutex sync.Mutex
	toCall func(reader *bufio.Reader, delim byte) ([]byte, error)
}

func (stub *ReaderReadBytesObj) UseFunction(toCall func(reader *bufio.Reader, delim byte) ([]byte, error)) {
	stub.toCallReadMutex.Lock()
	defer stub.toCallReadMutex.Unlock()


	stub.changedCallMutex.Lock()
	stub.toCall = toCall
}

func (stub *ReaderReadBytesObj) Execute(reader *bufio.Reader, delim byte) ([]byte, error) {
	stub.toCallReadMutex.Lock()
	defer stub.toCallReadMutex.Unlock()

	if stub.toCall == nil {
		stub.changedCallMutex.Lock()
		defer stub.changedCallMutex.Unlock()
		return reader.ReadBytes(delim)
	}
	return stub.toCall(reader, delim)
}

func (stub *ReaderReadBytesObj) Reset() {
	stub.toCallReadMutex.Lock()
	defer stub.toCallReadMutex.Unlock()

	stub.toCall = nil
	stub.changedCallMutex.Unlock()
}

// -------------------- ioutil.ReadAll
type IoutilReadAllObj struct {
	changedCallMutex sync.Mutex

	toCallReadMutex sync.Mutex
	toCall func(r io.Reader) ([]byte, error)
}

func (stub *IoutilReadAllObj) UseFunction(toCall func(r io.Reader) ([]byte, error)) {
	stub.toCallReadMutex.Lock()
	defer stub.toCallReadMutex.Unlock()


	stub.changedCallMutex.Lock()
	stub.toCall = toCall
}

func (stub *IoutilReadAllObj) Execute(r io.Reader) ([]byte, error) {
	stub.toCallReadMutex.Lock()
	defer stub.toCallReadMutex.Unlock()

	if stub.toCall == nil {
		stub.changedCallMutex.Lock()
		defer stub.changedCallMutex.Unlock()
		return ioutil.ReadAll(r)
	}
	return stub.toCall(r)
}

func (stub *IoutilReadAllObj) Reset() {
	stub.toCallReadMutex.Lock()
	defer stub.toCallReadMutex.Unlock()

	stub.toCall = nil
	stub.changedCallMutex.Unlock()
}
