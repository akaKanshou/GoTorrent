package internal

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
	"go.uber.org/zap"
)

func (tor *Torrent) ExtractTorrentInfo(path, dest string) error {
	// To implement: check if already exists

	path, dest = filepath.Clean(path), filepath.Clean(dest)
	var _, torrentName string = filepath.Split(path)

	fileExt := filepath.Ext(torrentName)
	if fileExt != ".torrent" {
		return errors.New("file extension not supported")
	}

	torrentName = strings.TrimSuffix(torrentName, fileExt)
	if torrentName == "" {
		torrentName = fmt.Sprintf("Torrent_%d", rand.Int63())
	}

	torrentFileReader, err := os.Open(path)
	if err != nil {
		return err
	}
	decoder := bencoder.NewDecoder(torrentFileReader)

	metaInfoMap, err := decoder.Decode()
	if err != nil {
		return err
	}

	tor.torrPath = path
	tor.destPath = dest
	tor.localData.infoDict.fileName = torrentName

	err = tor.unmarshalBencode(metaInfoMap)
	if err != nil {
		return err
	}
	return err
}

// parses all local data in torrent files
// returns an error only if it is blocking, otherwise logs the error
func (tor *Torrent) unmarshalBencode(metaInfoMap bencoder.BencodeDicts) error {
	var err error

	if err = tor.unmarshalTrackerData(metaInfoMap); err != nil {
		return err
	}

	if err = tor.unmarshalMetaInfo(metaInfoMap); err != nil {
		return err
	}

	torrentInfoDictCheck, ok := metaInfoMap["info"]
	if !ok {
		return MetaInfoMissingField{Field: "info"}
	}
	torrentInfoDict, ok := torrentInfoDictCheck.(bencoder.BencodeDicts)
	if !ok {
		return MetaInfoInvalidValueType{
			field:    "info",
			wantType: "map[string]BencodeDicts",
			haveType: reflect.TypeOf(torrentInfoDictCheck).String(),
		}
	}

	if err = tor.unmarshalInfoDict(torrentInfoDict); err != nil {
		return err
	}

	return nil
}

func (tor *Torrent) unmarshalTrackerData(metaInfoMap bencoder.BencodeDicts) error {
	//use announce-list if trackers responding, otherwise use announce
	//https://bittorrent.org/beps/bep_0012.html
	if announceListsListCheck, ok := metaInfoMap["announce-list"]; ok {
		announceListsList, ok := announceListsListCheck.(bencoder.BencodeLists)
		if !ok {
			return MetaInfoInvalidValueType{field: "announce list", wantType: "[][]string",
				haveType: reflect.TypeOf(announceListsListCheck).String()}
		}

		for _, announceList := range announceListsList {
			announceListURLs, ok := announceList.(bencoder.BencodeLists)
			if !ok {
				return MetaInfoInvalidValueType{field: "announce list", wantType: "[]string", haveType: reflect.TypeOf(announceListURLs).String()}
			}

			var announceListURLTier []string
			for _, announceListURL := range announceListURLs {
				announceURL, ok := announceListURL.(bencoder.BencodeBytes)
				if !ok {
					return MetaInfoInvalidValueType{field: "announce list", wantType: "string", haveType: reflect.TypeOf(announceURL).String()}
				}

				announceListURLTier = append(announceListURLTier, string(announceURL))
			}
			tor.localData.trackers.announceList = append(tor.localData.trackers.announceList, announceListURLTier)
		}
	} else {
		LogInfo("No announce-list field")
	}

	if announceURLCheck, ok := metaInfoMap["announce"]; ok {
		announceURL, ok := announceURLCheck.(bencoder.BencodeBytes)
		if !ok {
			return MetaInfoInvalidValueType{field: "announce", wantType: "string", haveType: reflect.TypeOf(announceURL).String()}
		}

		tor.localData.trackers.announce = string(announceURL)
	} else {
		return MetaInfoMissingField{Field: "announce"}
	}

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
	}

	createdByCheck, ok := metaInfoMap["created by"]
	if ok {
		createdBy, ok := createdByCheck.(bencoder.BencodeBytes)
		if !ok {
			return MetaInfoInvalidValueType{field: "created by", wantType: "string", haveType: reflect.TypeOf(createdByCheck).String()}
		}
		tor.localData.metaInfo.createdBy = string(createdBy)
	}

	encodingCheck, ok := metaInfoMap["encoding"]
	if ok {
		encoding, ok := encodingCheck.(bencoder.BencodeBytes)
		if !ok {
			return MetaInfoInvalidValueType{field: "encoding", wantType: "string", haveType: reflect.TypeOf(encodingCheck).String()}
		}
		tor.localData.metaInfo.encoding = string(encoding)
	}

	creationDateCheck, ok := metaInfoMap["creation date"]
	if ok {
		creationDate, ok := creationDateCheck.(bencoder.BencodeInts)
		if !ok {
			return MetaInfoInvalidValueType{field: "creation date", wantType: "int64",
				haveType: reflect.TypeOf(creationDateCheck).String()}
		}
		tor.localData.metaInfo.creationDate = int64(creationDate)
	}

	return nil
}

func (tor *Torrent) unmarshalInfoDict(torrentInfoDict bencoder.BencodeDicts) error {
	var err error

	err = tor.getInfoHash(torrentInfoDict)
	if err != nil {
		return err
	}

	pieceLengthCheck, ok := torrentInfoDict["piece length"]
	if !ok {
		return MetaInfoMissingField{Field: "piece length"}
	}
	pieceLength, ok := pieceLengthCheck.(bencoder.BencodeInts)
	if !ok {
		return MetaInfoInvalidValueType{field: "piece length", wantType: "int64", haveType: reflect.TypeOf(pieceLengthCheck).String()}
	}
	tor.localData.infoDict.pieceLength = uint64(pieceLength)

	privateCheck, ok := torrentInfoDict["private"]
	if ok {
		private, ok := privateCheck.(bencoder.BencodeInts)
		if !ok {
			return MetaInfoInvalidValueType{field: "private", wantType: "int64", haveType: reflect.TypeOf(privateCheck).String()}
		}

		switch private {
		case 1:
			tor.localData.infoDict.private = true
			break

		default:
			LogWarn(
				"Field info:private has invalid value. Setting private to false.",
				zap.String("field", "private"),
				zap.Int64("value", int64(private)),
			)
		case 0:
			tor.localData.infoDict.private = false
		}
	} else {
		tor.localData.infoDict.private = false
	}

	err = tor.getFileInfo(torrentInfoDict)
	if err != nil {
		return err
	}

	piecesCheck, ok := torrentInfoDict["pieces"]
	if !ok {
		return MetaInfoMissingField{Field: "pieces"}
	}
	pieces, ok := piecesCheck.(bencoder.BencodeBytes)
	if !ok {
		return MetaInfoInvalidValueType{field: "pieces", wantType: "string", haveType: reflect.TypeOf(piecesCheck).String()}
	}
	err = tor.getPieces(pieces)
	if err != nil {
		return err
	}

	return nil
}

// populates infoDict of torrent file struct
func (tor *Torrent) getFileInfo(torrentInfoDict bencoder.BencodeDicts) error {
	fileNameCheck, ok := torrentInfoDict["name"]
	if !ok {
	}
	fileName, ok := fileNameCheck.(bencoder.BencodeBytes)
	if !ok {

	}
	tor.localData.infoDict.fileName = string(fileName)

	multipleFilesList, ok := torrentInfoDict["files"]
	filesInfo := multipleFilesList.(bencoder.BencodeLists)
	if ok {
		tor.localData.infoDict.multipleFiles = true
		for _, file := range filesInfo {
			tor.localData.infoDict.fileSpecs = append(tor.localData.infoDict.fileSpecs,
				parseFiles(file, true))
		}
	} else {
		tor.localData.infoDict.multipleFiles = false
		tor.localData.infoDict.fileSpecs = append(tor.localData.infoDict.fileSpecs,
			parseFiles(torrentInfoDict, false))
	}

	return nil
}

func parseFiles(files bencoder.BencodingTypes, multipleFiles bool) fileSpecs {
	var fileSpecs fileSpecs
	fileDict, ok := files.(bencoder.BencodeDicts)
	if !ok {
	}

	fileSpecsLengthCheck, ok := fileDict["length"]
	if !ok {
	}
	fileSpecsLength, ok := fileSpecsLengthCheck.(bencoder.BencodeInts)
	fileSpecs.length = int64(fileSpecsLength)

	md5Sum, ok := fileDict["md5Sum"]
	if ok {
		md5SumStr, ok := md5Sum.(bencoder.BencodeBytes)
		if !ok {
		}
		fileSpecs.md5sum = [32]byte(md5SumStr)
	}
	if !ok {
	}
	if multipleFiles {
		filePathsCheck, ok := fileDict["path"]
		if !ok {
		}
		filePaths, ok := filePathsCheck.(bencoder.BencodeLists)
		if !ok {
		}
		var filePath []string
		for _, filePathStr := range filePaths {
			filePathStrCheck, ok := filePathStr.(bencoder.BencodeBytes)
			if !ok {
			}
			filePath = append(filePath, string(filePathStrCheck))
		}
		fileSpecs.path = filePath
	}
	return fileSpecs
}

// encodes infoDict and sets torrent infoHash
func (tor *Torrent) getInfoHash(infoDict bencoder.BencodeDicts) error {
	b, err := bencoder.NewEncoder().Encode(infoDict)
	if err != nil {
		return err
	}
	tor.localData.infoHash = sha1.Sum(b)
	return nil
}

// populates torrent pieces
func (tor *Torrent) getPieces(pieces bencoder.BencodeBytes) error {
	if len(pieces)%20 > 0 {
		return fmt.Errorf("invalid pieces")
	}

	for i := 0; i < len(pieces); i += 20 {
		tor.localData.infoDict.pieces = append(tor.localData.infoDict.pieces, hashBytes(pieces[i:i+20]))
	}

	return nil
}
