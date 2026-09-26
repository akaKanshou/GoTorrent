package internal

import (
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

	piecesDone       *bitfield.BitField
	piecesInProgress map[int64]peerID
	mutex            sync.Mutex
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

	var curOffset int64 = 0
	for i, fileSpec := range files.fileSpecs {
		f.files[i].filePath = filepath.Join(f.destination, filepath.Join(fileSpec.path...))
		f.files[i].pieceBegin = curOffset / int64(files.pieceLength)
		f.files[i].offsetBegin = curOffset - int64(files.pieceLength)*f.files[i].pieceBegin
		f.files[i].length = fileSpec.length

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

// allocatePieceResponsibility allocates responsibility of pieces to a peer.
// Uses a mutex for thread-safety
func (f *fileManager) allocatePieceResponsibility(numPieces int, rBitfield *bitfield.BitField, id peerID) ([]int64, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	pieces := make([]int64, 0, numPieces)
	for i := range f.piecesDone.Size() {
		if isPieceDone, err := f.piecesDone.TestBit(i); err != nil {
			return nil, err
		} else if isPieceDone {
			continue
		} else if peerHasPiece, err := rBitfield.TestBit(i); err != nil {
			return nil, err
		} else if !peerHasPiece {
			continue
		}

		pieces = append(pieces, int64(i))
		f.piecesInProgress[int64(i)] = id
		if len(pieces) == numPieces {
			break
		}
	}

	return pieces, nil
}

func (f *fileManager) writePiece(piece, length int64, buf []byte) error {
	if piece < 0 || length < 0 {
		return invalidPieceOffsetOrLengthError
	}

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

func (f *fileManager) deallocatePieceResponsibility(piece int64) error {
	_, ok := f.piecesInProgress[piece]
	if !ok {
		return pieceNotAllocated
	}
	delete(f.piecesInProgress, piece)
	return nil
}

func (f *fileManager) freePeerResponsibility(id peerID) {
	for pieceIndex, pieceRes := range f.piecesInProgress {
		if pieceRes == id {
			delete(f.piecesInProgress, pieceIndex)
		}
	}
}

var (
	invalidPieceOffsetOrLengthError = errors.New("invalid piece, offset or length")
	pieceOutOfRangeError            = errors.New("pieceOutOfRangeError")
	pieceNotAllocated               = errors.New("pieceNotAllocated")
)
