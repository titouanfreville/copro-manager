package internal

import (
	"errors"
	"strconv"

	"github.com/google/uuid"
)

var (
	// ErrUnsupportedType indicates the interface type is not supported for conversion.
	ErrUnsupportedType = errors.New("unsupported type")

	// ErrInvalidTypeConversion indicates the data provided cannot be converted to the expected type.
	ErrInvalidTypeConversion = errors.New("invalid type conversion")
)

// SetTyped sets the target interface value to the converted string value.
func SetTyped(val string, valList []string, to interface{}) error { //nolint:gocyclo
	switch typedTo := to.(type) {
	case *string:
		*typedTo = val
	case *int:
		tmp, err := strconv.Atoi(val)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = tmp
	case *int8:
		tmp, err := strconv.ParseInt(val, 10, 8)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = int8(tmp)
	case *int16:
		tmp, err := strconv.ParseInt(val, 10, 16)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = int16(tmp)
	case *int32:
		tmp, err := strconv.ParseInt(val, 10, 32)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = int32(tmp)
	case *int64:
		tmp, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = tmp
	case *uint:
		tmp, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = uint(tmp)
	case *uint8:
		tmp, err := strconv.ParseUint(val, 10, 8)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = uint8(tmp)
	case *uint16:
		tmp, err := strconv.ParseUint(val, 10, 16)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = uint16(tmp)
	case *uint32:
		tmp, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = uint32(tmp)
	case *uint64:
		tmp, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = tmp
	case *bool:
		tmp, err := strconv.ParseBool(val)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = tmp
	case *float32:
		tmp, err := strconv.ParseFloat(val, 32)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = float32(tmp)
	case *float64:
		tmp, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = tmp
	case *[]string:
		*typedTo = valList
	case *uuid.UUID:
		tmp, err := uuid.Parse(val)
		if err != nil {
			return ErrInvalidTypeConversion
		}
		*typedTo = tmp
	default:
		return ErrUnsupportedType
	}

	return nil
}
