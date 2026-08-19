package internal

// TODO: fix getting 'name' key in multiple files by maybe splitting the function

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gitlab.com/LUwUcifer/bencoder"
	"gitlab.com/LUwUcifer/gotorrent/logger"
	"go.uber.org/zap"
)

func (tor *Torrent) ExtractTorrentInfo(path, dest string) error {
	// To implement: check if already exists

	path, dest = filepath.Clean(path), filepath.Clean(dest)
	var _, torrentName string = filepath.Split(path)

	fileExt := filepath.Ext(torrentName)
	if fileExt != ".torrent" {
		logger.Logger.Error("Invalid file extension", zap.String("fileExt", fileExt))
		return errors.New("file extension not supported")
	}

	torrentName = strings.TrimSuffix(torrentName, fileExt)
	if torrentName == "" {
		torrentName = fmt.Sprintf("Torrent_%d", rand.Int63())
		logger.Logger.Warn("Torrent name is empty", zap.String("generatedName", torrentName))
	}

	torrentFileReader, err := os.Open(path)
	if err != nil {
		logger.Logger.Error("Error opening torrent file", zap.String("path", path), zap.Error(err))
		return err
	}
	decoder := bencoder.NewDecoder(torrentFileReader)

	metaInfoMap, err := decoder.Decode()
	if err != nil {
		logger.Logger.Error("Error decoding torrent file", zap.String("torrentName", torrentName), zap.Error(err))
		return err
	}

	tor.torrPath = path
	tor.destPath = dest
	tor.torrName = torrentName
	tor.localData.infoDict.fileName = torrentName

	err = tor.unmarshalBencode(metaInfoMap)
	if err != nil {
		logger.Logger.Error("Error unmarshaling torrent file", zap.String("torrentName", torrentName), zap.Error(err))
		return err
	}

	logger.Logger.Info("Added torrent", zap.String("torrentName", torrentName), zap.String("destPath", tor.destPath),
		zap.ByteString("infoHash", tor.localData.infoHash[:]))

	return nil
}

// parses all local data in torrent files
// returns an error only if it is blocking, otherwise logs the error
func (tor *Torrent) unmarshalBencode(metaInfoMap bencoder.BencodeDicts) error {
	var err error

	if err = tor.unmarshalTrackerData(metaInfoMap); err != nil {
		logger.Logger.Error("Error unmarshaling tracker data", zap.String("torrentName", tor.torrName))
		return err
	}

	if err = tor.unmarshalMetaInfo(metaInfoMap); err != nil {
		logger.Logger.Error("Error unmarshaling meta info", zap.String("torrentName", tor.torrName))
		return err
	}

	torrentInfoDictCheck, ok := metaInfoMap["info"]
	if !ok {
		logger.Logger.Error("Missing 'info' key", zap.String("torrentName", tor.torrName))
		return MetaInfoMissingField{Field: "info"}
	}
	torrentInfoDict, ok := torrentInfoDictCheck.(bencoder.BencodeDicts)
	if !ok {
		logger.Logger.Error("Invalid value in 'info' key", zap.String("torrentName", tor.torrName))
		return MetaInfoInvalidValueType{
			field:    "info",
			wantType: "map[string]BencodeDicts",
			haveType: reflect.TypeOf(torrentInfoDictCheck).String(),
		}
	}

	if err = tor.unmarshalInfoDict(torrentInfoDict); err != nil {
		logger.Logger.Error("Error unmarshaling info dict", zap.String("torrentName", tor.torrName))
		return err
	}

	logger.Logger.Info("Unmarshalled torrent meta data", zap.String("torrentName", tor.torrName))
	return nil
}

func (tor *Torrent) unmarshalTrackerData(metaInfoMap bencoder.BencodeDicts) error {
	//use announce-list if trackers responding, otherwise use announce
	//https://bittorrent.org/beps/bep_0012.html
	if announceListsListCheck, ok := metaInfoMap["announce-list"]; ok {
		announceListsList, ok := announceListsListCheck.(bencoder.BencodeLists)
		if !ok {
			logger.Logger.Error("Missing 'announce-list' key", zap.String("torrentName", tor.torrName))
			return MetaInfoInvalidValueType{field: "announce list", wantType: "[][]string",
				haveType: reflect.TypeOf(announceListsListCheck).String()}
		}

		for _, announceList := range announceListsList {
			announceListURLs, ok := announceList.(bencoder.BencodeLists)
			if !ok {
				logger.Logger.Error("Invalid value in 'announce-list' key", zap.String("torrentName", tor.torrName))
				return MetaInfoInvalidValueType{field: "announce list", wantType: "[]string", haveType: reflect.TypeOf(announceListURLs).String()}
			}

			var announceListURLTier []string
			for _, announceListURL := range announceListURLs {
				announceURL, ok := announceListURL.(bencoder.BencodeBytes)
				if !ok {
					logger.Logger.Error("Invalid value in 'announce-list' key", zap.String("torrentName", tor.torrName))
					return MetaInfoInvalidValueType{field: "announce list", wantType: "string", haveType: reflect.TypeOf(announceURL).String()}
				}

				announceListURLTier = append(announceListURLTier, string(announceURL))
			}
			tor.localData.trackers.announceList = append(tor.localData.trackers.announceList, announceListURLTier)
		}
	} else {
		logger.Logger.Info("No announce-list field")
	}

	if announceURLCheck, ok := metaInfoMap["announce"]; ok {
		announceURL, ok := announceURLCheck.(bencoder.BencodeBytes)
		if !ok {
			logger.Logger.Error("Missing 'announce' key", zap.String("torrentName", tor.torrName))
			return MetaInfoInvalidValueType{field: "announce", wantType: "string", haveType: reflect.TypeOf(announceURL).String()}
		}

		tor.localData.trackers.announce = string(announceURL)
	} else {
		logger.Logger.Error("Missing 'announce' key", zap.String("torrentName", tor.torrName))
		return MetaInfoMissingField{Field: "announce"}
	}

	logger.Logger.Info("Unmarshalled torrent tracker data", zap.String("torrentName", tor.torrName))
	return nil
}

func (tor *Torrent) unmarshalMetaInfo(metaInfoMap bencoder.BencodeDicts) error {
	commentCheck, ok := metaInfoMap["comment"]
	if ok {
		comment, ok := commentCheck.(bencoder.BencodeBytes)
		if !ok {
			return MetaInfoInvalidValueType{field: "comment", wantType: "string",
				haveType: reflect.TypeOf(commentCheck).String()}
		}
		tor.localData.metaInfo.comment = string(comment)
	} else {
		logger.Logger.Info("Missing 'comment' key", zap.String("torrentName", tor.torrName))
	}

	createdByCheck, ok := metaInfoMap["created by"]
	if ok {
		createdBy, ok := createdByCheck.(bencoder.BencodeBytes)
		if !ok {
			return MetaInfoInvalidValueType{field: "created by", wantType: "string", haveType: reflect.TypeOf(createdByCheck).String()}
		}
		tor.localData.metaInfo.createdBy = string(createdBy)
	} else {
		logger.Logger.Info("Missing 'created by' key", zap.String("torrentName", tor.torrName))
	}

	encodingCheck, ok := metaInfoMap["encoding"]
	if ok {
		encoding, ok := encodingCheck.(bencoder.BencodeBytes)
		if !ok {
			return MetaInfoInvalidValueType{field: "encoding", wantType: "string", haveType: reflect.TypeOf(encodingCheck).String()}
		}
		tor.localData.metaInfo.encoding = string(encoding)
	} else {
		logger.Logger.Info("Missing 'encoding' key", zap.String("torrentName", tor.torrName))
	}

	creationDateCheck, ok := metaInfoMap["creation date"]
	if ok {
		creationDate, ok := creationDateCheck.(bencoder.BencodeInts)
		if !ok {
			return MetaInfoInvalidValueType{field: "creation date", wantType: "int64",
				haveType: reflect.TypeOf(creationDateCheck).String()}
		}
		tor.localData.metaInfo.creationDate = int64(creationDate)
	} else {
		logger.Logger.Info("Missing 'creation date' key", zap.String("torrentName", tor.torrName))
	}

	logger.Logger.Info("Unmarshalled torrent meta info", zap.String("torrentName", tor.torrName))
	return nil
}

func (tor *Torrent) unmarshalInfoDict(torrentInfoDict bencoder.BencodeDicts) error {
	var err error

	err = tor.getInfoHash(torrentInfoDict)
	if err != nil {
		logger.Logger.Error("Failed to get info hash", zap.String("torrentName", tor.torrName))
		return err

	}

	pieceLengthCheck, ok := torrentInfoDict["piece length"]
	if !ok {
		logger.Logger.Error("Missing 'piece length' key", zap.String("torrentName", tor.torrName))
		return MetaInfoMissingField{Field: "piece length"}
	}
	pieceLength, ok := pieceLengthCheck.(bencoder.BencodeInts)
	if !ok {
		logger.Logger.Error("Invalid value in 'piece length' key", zap.String("torrentName", tor.torrName))
		return MetaInfoInvalidValueType{field: "piece length", wantType: "int64", haveType: reflect.TypeOf(pieceLengthCheck).String()}
	}
	tor.localData.infoDict.pieceLength = uint64(pieceLength)

	privateCheck, ok := torrentInfoDict["private"]
	if ok {
		private, ok := privateCheck.(bencoder.BencodeInts)
		if !ok {
			logger.Logger.Error("Invalid value in 'private' key", zap.String("torrentName", tor.torrName))
			return MetaInfoInvalidValueType{field: "private", wantType: "int64", haveType: reflect.TypeOf(privateCheck).String()}
		}

		if private == 1 {
			tor.localData.infoDict.private = true

			logger.Logger.Info("Torrent is private", zap.String("torrentName", tor.torrName))
		} else {
			tor.localData.infoDict.private = false

			if private != 0 {
				logger.Logger.Warn("Invalid value in 'private' key, defaulting to public", zap.String("torrentName",
					tor.torrName), zap.Int64("private", int64(private)))
			}
		}
	} else {
		logger.Logger.Info("Missing 'private' key", zap.String("torrentName", tor.torrName))
		tor.localData.infoDict.private = false
	}

	err = tor.getFileInfo(torrentInfoDict)
	if err != nil {
		logger.Logger.Error("Failed to get file info", zap.String("torrentName", tor.torrName))
		return err
	}

	piecesCheck, ok := torrentInfoDict["pieces"]
	if !ok {
		logger.Logger.Error("Missing 'pieces' key", zap.String("torrentName", tor.torrName))
		return MetaInfoMissingField{Field: "pieces"}
	}
	pieces, ok := piecesCheck.(bencoder.BencodeBytes)
	if !ok {
		logger.Logger.Error("Invalid value in 'pieces' key", zap.String("torrentName", tor.torrName))
		return MetaInfoInvalidValueType{field: "pieces", wantType: "string", haveType: reflect.TypeOf(piecesCheck).String()}
	}
	err = tor.getPieces(pieces)
	if err != nil {
		logger.Logger.Error("Failed to get pieces", zap.String("torrentName", tor.torrName))
		return err
	}

	logger.Logger.Info("Unmarshalled torrent info dict", zap.String("torrentName", tor.torrName))
	return nil
}

// populates infoDict of torrent file struct
func (tor *Torrent) getFileInfo(torrentInfoDict bencoder.BencodeDicts) error {
	fileNameCheck, ok := torrentInfoDict["name"]
	if !ok {
		logger.Logger.Error("Missing 'name' key", zap.String("torrentName", tor.torrName))
		return MetaInfoMissingField{Field: "name"}
	}
	fileName, ok := fileNameCheck.(bencoder.BencodeBytes)
	if !ok {
		logger.Logger.Error("Invalid value in 'name' key", zap.String("torrentName", tor.torrName))
		return MetaInfoInvalidValueType{field: "name", wantType: "string", haveType: reflect.TypeOf(fileNameCheck).String()}
	}
	tor.localData.infoDict.fileName = string(fileName)

	multipleFilesList, ok := torrentInfoDict["files"]
	filesInfo := multipleFilesList.(bencoder.BencodeLists)
	if ok {
		logger.Logger.Info("Torrent is multi-file", zap.String("torrentName", tor.torrName))

		tor.localData.infoDict.multipleFiles = true
		for i, file := range filesInfo {
			fileSpec, err := tor.parseFiles(file)
			if err != nil {
				logger.Logger.Error("Failed to parse files", zap.String("torrentName", tor.torrName), zap.Int("fileIndex",
					i))
				return err
			}
			tor.localData.infoDict.fileSpecs = append(tor.localData.infoDict.fileSpecs,
				fileSpec)
		}
	} else {
		logger.Logger.Info("Torrent is single-file", zap.String("torrentName", tor.torrName))
		tor.localData.infoDict.multipleFiles = false
		fileSpec, err := tor.parseFiles(torrentInfoDict)
		if err != nil {
			logger.Logger.Error("Failed to parse files", zap.String("torrentName", tor.torrName), zap.Error(err))
			return err
		}
		tor.localData.infoDict.fileSpecs = append(tor.localData.infoDict.fileSpecs, fileSpec)
	}

	logger.Logger.Info("Unmarshalled torrent file info", zap.String("torrentName", tor.torrName))
	return nil
}

func (tor *Torrent) parseFiles(files bencoder.BencodingTypes) (fileSpecs, error) {
	var fileSpecs fileSpecs
	fileDict, ok := files.(bencoder.BencodeDicts)
	if !ok {
		logger.Logger.Error("Invalid value in files", zap.String("torrentName", tor.torrName))
		return fileSpecs, MetaInfoInvalidValueType{field: "files", wantType: "string", haveType: reflect.TypeOf(files).String()}
	}

	if !tor.localData.infoDict.multipleFiles {
		fileSpecsNameCheck, ok := fileDict["name"]
		if !ok {
			logger.Logger.Error("Missing 'name' key", zap.String("torrentName", tor.torrName))
			return fileSpecs, MetaInfoMissingField{Field: "name"}
		}
		fileSpecsName, ok := fileSpecsNameCheck.(bencoder.BencodeBytes)
		if !ok {
			logger.Logger.Error("Invalid value in files", zap.String("torrentName", tor.torrName))
			return fileSpecs, MetaInfoInvalidValueType{
				field:    "name",
				wantType: "string",
				haveType: reflect.TypeOf(fileSpecsNameCheck).String()}
		}
		fileSpecs.path = []string{string(fileSpecsName)}
	}

	fileSpecsLengthCheck, ok := fileDict["length"]
	if !ok {
		logger.Logger.Error("Missing 'length' key", zap.String("torrentName", tor.torrName))
		return fileSpecs, MetaInfoMissingField{Field: "length"}
	}
	fileSpecsLength, ok := fileSpecsLengthCheck.(bencoder.BencodeInts)
	fileSpecs.length = int64(fileSpecsLength)

	md5Sum, ok := fileDict["md5Sum"]
	if ok {
		md5SumStr, ok := md5Sum.(bencoder.BencodeBytes)
		if !ok {
			logger.Logger.Error("Invalid value in 'md5Sum' key", zap.String("torrentName", tor.torrName))
			return fileSpecs, MetaInfoInvalidValueType{field: "md5Sum", wantType: "string",
				haveType: reflect.TypeOf(md5SumStr).String()}
		}
		fileSpecs.md5sum = [32]byte(md5SumStr)
	} else {
		logger.Logger.Info("No 'md5Sum' key", zap.String("torrentName", tor.torrName))
	}

	if tor.localData.infoDict.multipleFiles {
		filePathsCheck, ok := fileDict["path"]
		if !ok {
			logger.Logger.Error("Missing 'path' key", zap.String("torrentName", tor.torrName))
			return fileSpecs, MetaInfoMissingField{Field: "path"}
		}
		filePaths, ok := filePathsCheck.(bencoder.BencodeLists)
		if !ok {
			logger.Logger.Error("Invalid value in path", zap.String("torrentName", tor.torrName))
			return fileSpecs, MetaInfoInvalidValueType{field: "path", wantType: "[]string",
				haveType: reflect.TypeOf(filePathsCheck).String()}
		}

		for _, filePathStr := range filePaths {
			filePathStrCheck, ok := filePathStr.(bencoder.BencodeBytes)
			if !ok {
				logger.Logger.Error("Invalid value in path", zap.String("torrentName", tor.torrName))
				return fileSpecs, MetaInfoInvalidValueType{field: "path", wantType: "string", haveType: reflect.TypeOf(filePathStrCheck).String()}
			}
			fileSpecs.path = append(fileSpecs.path, string(filePathStrCheck))
		}
	}

	logger.Logger.Info("Parsed file", zap.String("torrentName", tor.torrName), zap.Strings("fileName", fileSpecs.path))
	return fileSpecs, nil
}

// encodes infoDict and sets torrent infoHash
func (tor *Torrent) getInfoHash(infoDict bencoder.BencodeDicts) error {
	b, err := bencoder.NewEncoder().Encode(infoDict)
	if err != nil {
		logger.Logger.Error("Error encoding torrent info dict", zap.Error(err))
		return err
	}
	tor.localData.infoHash = sha1.Sum(b)
	logger.Logger.Info("Info hash computed")
	return nil
}

// populates torrent pieces
func (tor *Torrent) getPieces(pieces bencoder.BencodeBytes) error {
	if len(pieces)%20 > 0 {
		logger.Logger.Error("Invalid piece length", zap.Int("pieces", len(pieces)))
		return errors.New("invalid pieces")
	}

	for i := 0; i < len(pieces); i += 20 {
		tor.localData.infoDict.pieces = append(tor.localData.infoDict.pieces, hashBytes(pieces[i:i+20]))
	}

	logger.Logger.Info("Pieces computed")
	return nil
}
