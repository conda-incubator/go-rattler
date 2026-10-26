//go:build cgo

package gorattler

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/conda-incubator/go-rattler/internal/api"
)

// Private numeric operation IDs. They must match the Rust dispatcher.
const (
	parsePrefixOperation     uint32 = api.OperationParsePrefix
	batchPrefixOperation     uint32 = api.OperationParsePrefixBatch
	minimalPrefixOperation   uint32 = api.OperationParseMinimalPrefix
	archiveOperation         uint32 = api.OperationArchiveFilename
	indexOperation           uint32 = api.OperationIndexJSON
	channelOperation         uint32 = api.OperationChannel
	matchSpecOperation       uint32 = api.OperationMatchSpec
	versionSpecOperation     uint32 = api.OperationVersionSpec
	environmentOperation     uint32 = api.OperationEnvironmentYAML
	explicitOperation        uint32 = api.OperationExplicitSpec
	subdirOperation          uint32 = api.OperationOCISubdir
	collectPrefixOperation   uint32 = api.OperationCollectPrefix
	lockPackagesOperation    uint32 = api.OperationLockPackages
	lockSyncOperation        uint32 = api.OperationLockSync
	compareVersionsOperation uint32 = api.OperationCompareVersions
)

// callJSON marshals request, runs one native operation, and decodes the
// response into result. Unknown response fields are ignored.
func callJSON(operation uint32, request any, result any) error {
	input, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("%w: encode request: %v", ErrInvalidInput, err)
	}
	output, err := api.JSON(operation, input)
	if err != nil {
		var status *api.Status
		if errors.As(err, &status) {
			return decodeNativeError(status)
		}
		return fmt.Errorf("%w: %v", ErrNative, err)
	}
	if err := json.Unmarshal(output, result); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrNative, err)
	}
	return nil
}

// decodeNativeError converts a non-success bridge status into *Error.
func decodeNativeError(status *api.Status) error {
	var envelope struct {
		Error string `json:"error"`
	}
	message := "no error detail"
	if json.Unmarshal(status.Body, &envelope) == nil && envelope.Error != "" {
		message = envelope.Error
	}
	return &Error{Operation: status.Operation, Status: status.Code, Message: message}
}
