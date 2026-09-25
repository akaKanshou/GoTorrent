package internal

import (
	"os"
	"path/filepath"
)

type fileManager struct {
	destination string
	files       []fileInfo
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
