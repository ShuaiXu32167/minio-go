/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage (C) 2015 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package minio

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

// objectReadSeeker container for io.ReadSeeker.
type objectReadSeeker struct {
	// mutex.
	mutex *sync.Mutex

	api        API
	reader     io.ReadCloser
	isRead     bool
	stat       ObjectStat
	offset     int64
	bucketName string
	objectName string
}

// newObjectReadSeeker wraps getObject request returning a io.ReadSeeker.
func newObjectReadSeeker(api API, bucket, object string) *objectReadSeeker {
	return &objectReadSeeker{
		mutex:      new(sync.Mutex),
		reader:     nil,
		isRead:     false,
		api:        api,
		offset:     0,
		bucketName: bucket,
		objectName: object,
	}
}

// Read reads up to len(p) bytes into p.  It returns the number of bytes
// read (0 <= n <= len(p)) and any error encountered.  Even if Read
// returns n < len(p), it may use all of p as scratch space during the call.
// If some data is available but not len(p) bytes, Read conventionally
// returns what is available instead of waiting for more.
//
// When Read encounters an error or end-of-file condition after
// successfully reading n > 0 bytes, it returns the number of
// bytes read.  It may return the (non-nil) error from the same call
// or return the error (and n == 0) from a subsequent call.
// An instance of this general case is that a Reader returning
// a non-zero number of bytes at the end of the input stream may
// return either err == EOF or err == nil.  The next Read should
// return 0, EOF.
func (r *objectReadSeeker) Read(p []byte) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.isRead {
		reader, _, err := r.api.getObject(r.bucketName, r.objectName, r.offset, 0)
		if err != nil {
			return 0, err
		}
		r.reader = reader
		r.isRead = true
	}
	n, err := r.reader.Read(p)
	if err == io.EOF {
		// drain any remaining body, discard it before closing the body.
		io.Copy(ioutil.Discard, r.reader)
		r.reader.Close()
		return n, err
	}
	if err != nil {
		// drain any remaining body, discard it before closing the body.
		io.Copy(ioutil.Discard, r.reader)
		r.reader.Close()
		return 0, err
	}
	return n, nil
}

// Seek sets the offset for the next Read or Write to offset,
// interpreted according to whence: 0 means relative to the start of
// the file, 1 means relative to the current offset, and 2 means
// relative to the end. Seek returns the new offset relative to the
// start of the file and an error, if any.
//
// Seeking to an offset before the start of the file is an error.
// TODO: whence value of '1' and '2' are not implemented yet.
func (r *objectReadSeeker) Seek(offset int64, whence int) (int64, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.offset = offset
	return offset, nil
}

// Size returns the size of the object. If there is any error
// it will be of type ErrorResponse.
func (r *objectReadSeeker) Size() (int64, error) {
	objectSt, err := r.api.headObject(r.bucketName, r.objectName)
	r.stat = objectSt
	return r.stat.Size, err
}

// tempFile - temporary file container.
type tempFile struct {
	*os.File
	mutex *sync.Mutex
}

// newTempFile returns a new unused file.
func newTempFile(prefix string) (*tempFile, error) {
	// use platform specific temp directory.
	file, err := ioutil.TempFile(os.TempDir(), prefix)
	if err != nil {
		return nil, err
	}
	return &tempFile{
		File:  file,
		mutex: new(sync.Mutex),
	}, nil
}

// cleanupStaleTempFiles - cleanup any stale files present in temp directory at a prefix.
func cleanupStaleTempfiles(prefix string) error {
	globPath := filepath.Join(os.TempDir(), prefix) + "*"
	staleFiles, err := filepath.Glob(globPath)
	if err != nil {
		return err
	}
	for _, staleFile := range staleFiles {
		if err := os.Remove(staleFile); err != nil {
			return err
		}
	}
	return nil
}

// Close - closer wrapper to close and remove temporary file.
func (t *tempFile) Close() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.File != nil {
		// Close the file.
		if err := t.File.Close(); err != nil {
			return err
		}
		// Remove wrapped file.
		if err := os.Remove(t.File.Name()); err != nil {
			return err
		}
		t.File = nil
	}
	return nil
}