package rest

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"

	"github.com/titouanfreville/copro-manager/api/src/core/rest/internal"
)

const (
	// MaxBodySize is the maximum allowed request body size (32MB).
	MaxBodySize = 32 << 20
)

var (
	// ErrUnsupportedType indicates the interface type is not supported for conversion.
	ErrUnsupportedType = internal.ErrUnsupportedType

	// ErrInvalidTypeConversion indicates the data provided cannot be converted to the expected type.
	ErrInvalidTypeConversion = internal.ErrInvalidTypeConversion

	// ErrNotFound indicates a required parameter was not found.
	ErrNotFound = errors.New("not found")

	emptyUUID = uuid.UUID{}

	binder Binder = BindImpl{}
)

// BindImpl implements the Binder interface.
type BindImpl struct{}

// Bind provides a Binder singleton.
func Bind() Binder {
	return binder
}

func (bind BindImpl) Form(formData url.Values, obj interface{}) error {
	tmp := map[string]interface{}{}
	for k, v := range formData {
		tmp[k] = v[0]
	}

	return bind.weakDecodeHook(tmp, obj)
}

func (BindImpl) weakDecodeHook(input interface{}, output interface{}) error {
	config := &mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           output,
		WeaklyTypedInput: true,
		DecodeHook: func(from reflect.Type, to reflect.Type, v interface{}) (interface{}, error) {
			if from.Kind() == reflect.String && to == reflect.TypeOf(emptyUUID) {
				value, ok := v.(string)
				if !ok {
					return nil, ErrInvalidTypeConversion
				}

				return uuid.Parse(value)
			}

			return v, nil
		},
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}

func (bind BindImpl) RequestData(r *http.Request, dataList ...interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, MaxBodySize)

	if IsJSONData(r.Header) {
		body, _ := io.ReadAll(r.Body)
		if err := r.Body.Close(); err != nil {
			return err
		}

		for _, data := range dataList {
			if err := render.DecodeJSON(bytes.NewBuffer(body), data); err != nil {
				return err
			}
		}
	} else {
		if IsMultipartFormData(r.Header) {
			if err := r.ParseMultipartForm(MaxBodySize); err != nil { //#nosec G120 -- body bounded by MaxBytesReader above
				return err
			}
		} else if err := r.ParseForm(); err != nil {
			return err
		}

		for _, data := range dataList {
			if err := bind.Form(r.PostForm, data); err != nil {
				return err
			}
		}
	}

	return nil
}

func (bind BindImpl) FormData(r *http.Request, dataList ...interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, MaxBodySize)

	if IsMultipartFormData(r.Header) {
		if err := r.ParseMultipartForm(MaxBodySize); err != nil { //#nosec G120 -- body bounded by MaxBytesReader above
			return err
		}
	} else if err := r.ParseForm(); err != nil {
		return err
	}

	for _, data := range dataList {
		if err := bind.Form(r.PostForm, data); err != nil {
			return err
		}
	}

	return nil
}

func (BindImpl) JSONData(r *http.Request, dataList ...interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, MaxBodySize)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		_ = r.Body.Close()
		return err
	}
	if err := r.Body.Close(); err != nil {
		return err
	}

	for _, data := range dataList {
		if err := render.DecodeJSON(bytes.NewBuffer(body), data); err != nil {
			return err
		}
	}

	return nil
}

func (bind BindImpl) URLParams(r *http.Request, params map[string]interface{}) error {
	for key, val := range params {
		refVal := reflect.ValueOf(val)
		if refVal.Kind() == reflect.Ptr {
			if err := bind.URLParam(r, key, refVal.Elem().Addr().Interface()); err != nil {
				return err
			}
		}
	}

	return nil
}

func (BindImpl) URLParam(r *http.Request, key string, to interface{}) error {
	param := chi.URLParam(r, key)
	refTo := reflect.ValueOf(to)

	if param == "" {
		return ErrNotFound
	}

	if refTo.Kind() == reflect.Ptr {
		return internal.SetTyped(param, []string{}, refTo.Elem().Addr().Interface())
	}

	return internal.SetTyped(param, []string{}, to)
}

func (bind BindImpl) URLArgs(r *http.Request, params map[string]interface{}) error {
	for key, val := range params {
		refVal := reflect.ValueOf(val)
		if refVal.Kind() == reflect.Ptr {
			if err := bind.URLArg(r, key, refVal.Elem().Addr().Interface()); err != nil {
				return err
			}
		}
	}

	return nil
}

func (BindImpl) URLArg(r *http.Request, key string, to interface{}) error {
	args := r.URL.Query()[key]
	arg := r.URL.Query().Get(key)

	if arg == "" {
		return nil
	}

	refTo := reflect.ValueOf(to)
	if refTo.Kind() == reflect.Ptr {
		return internal.SetTyped(arg, args, refTo.Elem().Addr().Interface())
	}

	return internal.SetTyped(arg, args, to)
}
