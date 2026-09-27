package internal

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"go.loafoe.dev/bitfield/v2"
)

type fileManager struct {
	destination string
	files       []fileInfo
	pieceLength int64
	totalLength int64
	pieces      []hashBytes

	piecesDone       *bitfield.BitField
	piecesInProgress map[int64]*peer
	pieceBuffers     map[int64][]byte
	mutex            sync.Mutex
	piecesDownloaded int
}

type fileInfo struct {
	filePath    string
	pieceBegin  int64
	offsetBegin int64
	length      int64
}

// initFiles initializes the files of fileManager, as well as allocating space for them.
func (f *fileManager) initFiles(files infoDict) error {
	if files.multipleFiles {
		f.destination = filepath.Join(f.destination, files.fileName)
	}

	f.files = make([]fileInfo, len(files.fileSpecs))
	f.totalLength = 0
	f.piecesDownloaded = 0

	var curOffset int64 = 0
	for i, fileSpec := range files.fileSpecs {
		f.files[i].filePath = filepath.Join(f.destination, filepath.Join(fileSpec.path...))
		f.files[i].pieceBegin = curOffset / int64(files.pieceLength)
		f.files[i].offsetBegin = curOffset - int64(files.pieceLength)*f.files[i].pieceBegin
		f.files[i].length = fileSpec.length

		f.totalLength += fileSpec.length

		curOffset += fileSpec.length
	}

	for _, file := range f.files {
		err := os.MkdirAll(filepath.Dir(file.filePath), os.ModePerm)
		if err != nil {
			return err
		}

		fd, err := os.Create(file.filePath) // Creating file
		if err != nil {
			return err
		}
		fd.Close()

		err = os.Truncate(file.filePath, file.length) // Set file to correct length
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *fileManager) writePiece(piece, length int64, buf []byte) error {
	if piece < 0 || length < 0 {
		return invalidPieceOffsetOrLengthError
	}
	buf = buf[:length]

	firstFile := f.getFirstFileForPiece(piece)
	if firstFile == -1 {
		return pieceOutOfRangeError
	}

	byteOffset := piece * f.pieceLength

	for _, file := range f.files[firstFile:] {
		fileByteOffsetGlobal := file.pieceBegin*f.pieceLength + file.offsetBegin
		fileByteOffset := max(byteOffset-fileByteOffsetGlobal, 0)
		bytesWritten := max(file.length-fileByteOffset, length)

		err := f.writeToFile(&file, fileByteOffset, bytesWritten, buf)
		if err != nil {
			return err
		}

		buf = buf[bytesWritten:]
		length -= bytesWritten
		if len(buf) == 0 {
			break
		}
	}

	f.piecesDone.SetBit(uint64(piece))
	f.piecesDownloaded++
	return nil
}

func (f *fileManager) writeToFile(file *fileInfo, fileByteOffset, bytesWritten int64, buf []byte) error {
	fd, err := os.OpenFile(file.filePath, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()
	n, err := fd.WriteAt(buf[:bytesWritten], fileByteOffset)
	if err != nil {
		return err
	} else if n != int(bytesWritten) {
		return io.ErrShortWrite
	}

	return nil
}

// getFirstFileForPiece returns the index of the file which the contains the first byte of the given piece and offset.
// If the given piece and offset exceed all files, -1 is returned.
// Linear search implementation; TODO: Change to binary search
func (f *fileManager) getFirstFileForPiece(piece int64) int {
	for i, file := range f.files {
		if file.pieceBegin*f.pieceLength+file.offsetBegin+file.length > piece*f.pieceLength {
			return i
		}
	}

	return -1
}

func (f *fileManager) getPieceToDownload(available *bitfield.BitField) int64 {
	for index := uint64(0); index < available.Size(); index++ {
		iHavePiece, _ := f.piecesDone.TestBit(index)
		uHavePiece, _ := f.piecesDone.TestBit(index)
		if iHavePiece || !uHavePiece {
			continue
		}

		peerAssigned, ok := f.piecesInProgress[int64(index)]
		if ok && !peerAssigned.destroyed.Load() {
			continue
		}

		return int64(index)
	}
	return -1
}

func (f *fileManager) getPieceLength(index int64) int64 {
	if uint64(index+1) < f.piecesDone.Size() {
		return f.pieceLength
	}

	return f.totalLength - f.pieceLength*index
}

func (f *fileManager) verifyPiece(buffer []byte, index int64) bool {
	shaHash := sha1.Sum(buffer)
	return bytes.Compare(shaHash[:], f.pieces[index][:]) == 0
}

var (
	invalidPieceOffsetOrLengthError = errors.New("invalid piece, offset or length")
	pieceOutOfRangeError            = errors.New("pieceOutOfRangeError")
	pieceNotAllocated               = errors.New("pieceNotAllocated")
	shaHashFailed                   = errors.New("SHA1 hash of piece does not match")
)
