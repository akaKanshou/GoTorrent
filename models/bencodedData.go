package models

type announce []byte
type metaInfo struct {
	creationDate uint64
	comment      string
	createdBy    string
	encoding     string
	private      uint64
}

type fileLength uint64
type files struct {
	length fileLength
	path   string
}
type sizeInfo struct {
	fileLength    fileLength
	filesDict     []files
	multipleFiles bool
}

type infoDict struct {
	fileName    string
	pieceLength uint64
	pieces      [][]string //SHA1 hashes of every piece
	sizeInfo    sizeInfo
}
type bencodedData struct {
	announceList []announce
	metaInfo     metaInfo
	infoDict     infoDict
}
