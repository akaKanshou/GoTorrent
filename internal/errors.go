package internal

import (
	"fmt"
)

type MetaInfoMissingField struct {
	Field string
}

type MetaInfoInvalidValueType struct {
	field, wantType, haveType string
}

type MalformedAnnounceURl struct {
	announceURL string
	noProtocol  bool
}

type UnimplementedError struct {
	feature string
}

type TrackerBadResponse struct {
	expectedBytes, gotBytes int

	mismatchedFields string
}

type GenericError struct {
	reason string
}

func (err MetaInfoMissingField) Error() string {
	return fmt.Sprintf("missing mandatory field %v", err.Field)
}

func (err MetaInfoInvalidValueType) Error() string {
	return fmt.Sprintf("invalid torrent file. Expected type %v for field %v, got %v", err.wantType, err.field, err.haveType)
}

func (err GenericError) Error() string {
	return err.reason
}

func (err MalformedAnnounceURl) Error() string {
	if err.noProtocol {
		return fmt.Sprintf("no protocol defined in %v", err.announceURL)
	}

	return "malformed announce URL"
}

func (error TrackerBadResponse) Error() string {
	if error.expectedBytes > 0 {
		return fmt.Sprintf("expected %v bytes, got %v bytes", error.expectedBytes, error.gotBytes)
	}

	return fmt.Sprintf("mismatched field %v", error.mismatchedFields)
}

func (error UnimplementedError) Error() string {
	return fmt.Sprintf("%v is not yet implemented", error.feature)
}
