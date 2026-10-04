package internal

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"sync"

	"gitlab.com/akaKanshou/gotorrent/logger"
	"go.loafoe.dev/bitfield/v2"
	"go.uber.org/zap"
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

	progressFile string
}

type fileInfo struct {
	filePath    string
	pieceBegin  int64
	offsetBegin int64
	length      int64
}

func newFileManager(destination string, pieceLength int64, pieces []hashBytes, infoHash hashBytes) *fileManager {
	fileManager := &fileManager{
		destination:      filepath.Clean(destination),
		pieceLength:      pieceLength,
		pieces:           pieces,
		piecesDone:       bitfield.BigEndian.New(uint64(len(pieces))),
		piecesInProgress: make(map[int64]*peer),
		pieceBuffers:     make(map[int64][]byte),
		progressFile:     filepath.Join(destination, fmt.Sprintf("gt_download%X", infoHash)),
	}
	return fileManager
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
		f.files[i].offsetBegin = curOffset % int64(files.pieceLength)
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

	// TODO: Create a flag to enable/disable this feature
	// Init save progress file if it doesn't exist
	err := f.initSavedProgress()
	if err != nil {
		return err
	}

	// Load progress
	err = f.loadProgress()
	if err != nil {
		return err
	}

	return nil
}

func (f *fileManager) writePiece(piece, length int64, buf []byte) error {
	if piece < 0 || length < 0 || f.getPieceLength(piece) < length {
		logger.Logger.Error("Failed to write piece", zap.Error(invalidPieceOffsetOrLengthError))
		return invalidPieceOffsetOrLengthError
	}
	buf = buf[:length]

	firstFile := f.getFirstFileForPiece(piece)
	if firstFile == -1 {
		logger.Logger.Error("Failed to write piece", zap.Error(pieceOutOfRangeError))
		return pieceOutOfRangeError
	}

	byteOffset := piece * f.pieceLength

	for _, file := range f.files[firstFile:] {
		fileByteOffsetGlobal := file.pieceBegin*f.pieceLength + file.offsetBegin
		fileByteOffset := max(byteOffset-fileByteOffsetGlobal, 0)
		bytesWritten := min(file.length-fileByteOffset, length)

		logger.Logger.Debug("Writing to file", zap.String("file", file.filePath), zap.Int64("file begin index", file.pieceBegin),
			zap.Int64("file begin offset", file.offsetBegin), zap.Int64("file length", file.length), zap.Int64("write offset", fileByteOffset),
			zap.Int64("write length", bytesWritten))
		err := f.writeToFile(&file, fileByteOffset, bytesWritten, buf)
		if err != nil {
			logger.Logger.Error("Failed to write piece", zap.Error(err))
			return err
		}

		buf = buf[bytesWritten:]
		length -= bytesWritten
		if len(buf) == 0 {
			break
		}
	}

	logger.Logger.Debug("Piece written to file", zap.Int64("piece", piece))
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
		uHavePiece, _ := available.TestBit(index)
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

func (f *fileManager) initSavedProgress() error {
	fd, err := os.OpenFile(f.progressFile, os.O_WRONLY|os.O_EXCL|os.O_CREATE, 0600)
	if err != nil && errors.Is(err, os.ErrExist) {
		return nil
	} else if err != nil {
		return err
	}
	defer fd.Close()

	logger.Logger.Debug("Initiating saved progress", zap.String("file", f.progressFile))
	_, err = fd.Write(f.piecesDone.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func (f *fileManager) loadProgress() error {
	buf, err := f.getSavedData()
	if err != nil {
		return err
	}
	logger.Logger.Debug("Loading progress", zap.String("progress", fmt.Sprintf("%x", buf)))

	f.piecesDone = bitfield.BigEndian.FromBytes(buf, f.piecesDone.Size())
	return nil
}

func (f *fileManager) updateSavedProgress(index int64) error {
	fd, err := os.OpenFile(f.progressFile, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()

	relByte := index / 8 * 8
	_, err = fd.WriteAt(f.piecesDone.Bytes()[relByte:relByte+1], relByte)
	if err != nil {
		return err
	}
	return nil
}

func (f *fileManager) getSavedData() ([]byte, error) {
	fd, err := os.OpenFile(f.progressFile, os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	buf := make([]byte, (len(f.pieces)+7)/8)
	_, err = fd.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (f *fileManager) dataLeftToDownload() uint64 {
	bytesLeft := f.totalLength
	for index := range f.pieces {
		bytesLeft -= int64(bits.OnesCount8(f.piecesDone.Bytes()[index])) * f.pieceLength
	}
	bytesLeft += f.pieceLength - f.getPieceLength(int64(f.piecesDone.Size()-1))
	return uint64(bytesLeft)
}

// TODO: introduce caching pieces
func (f *fileManager) readBlockFromFile(index, begin, length int64) ([]byte, error) {
	if index < 0 || length < 0 || f.getPieceLength(index) < length {
		logger.Logger.Error("Failed to write piece", zap.Error(invalidPieceOffsetOrLengthError))
		return nil, invalidPieceOffsetOrLengthError
	}

	firstFile := f.getFirstFileForPiece(index)
	if firstFile == -1 {
		return nil, pieceOutOfRangeError
	}

	seek := int64(0)
	piece := make([]byte, length)

	byteOffset := index*f.pieceLength + begin

	for _, file := range f.files[firstFile:] {
		fileByteOffsetGlobal := file.pieceBegin*f.pieceLength + file.offsetBegin
		fileByteOffset := max(byteOffset-fileByteOffsetGlobal, 0)

		n, err := f.readFromFile(file, fileByteOffset, piece[seek:])
		if err != nil {
			return nil, err
		}

		seek += n
		if seek == length {
			break
		}
	}

	return piece, nil
}

func (f *fileManager) readFromFile(file fileInfo, offset int64, buffer []byte) (int64, error) {
	fd, err := os.OpenFile(file.filePath, os.O_RDONLY, 0600)
	if err != nil {
		return 0, err
	}
	defer fd.Close()

	n, err := fd.ReadAt(buffer, offset)
	if err != nil {
		return 0, err
	}

	return int64(n), err
}
