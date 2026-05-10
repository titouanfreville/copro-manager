package meters

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

type mockMeterReader struct{ mock.Mock }

func (m *mockMeterReader) ReadMeterPhoto(
	ctx context.Context,
	kind entities.MeterPhotoKind,
	image []byte,
	mimeType string,
) ([]float64, float64, error) {
	args := m.Called(ctx, kind, image, mimeType)
	var values []float64
	if v := args.Get(0); v != nil {
		values = v.([]float64)
	}
	return values, args.Get(1).(float64), args.Error(2)
}

func ucWithReader(r *mockMeterReader) *usecases {
	return &usecases{
		logger: zap.NewNop(),
		reader: r,
	}
}

// TestAnalyzeImage covers the four shapes the route observes: a clean
// global reading, a clean detail reading, a reader error (must NOT
// surface; UI falls back to manual entry), and an empty values
// response (no detection).
func TestAnalyzeImage(t *testing.T) {
	ctx := context.Background()
	bytes := []byte("fake-jpeg-bytes")

	Convey("Global photo: 1 value, confidence replicated into 1-slot slice", t, func() {
		r := &mockMeterReader{}
		r.On("ReadMeterPhoto", ctx, entities.MeterPhotoKindGlobal, bytes, "image/jpeg").
			Return([]float64{1234.567}, 0.92, nil)
		uc := ucWithReader(r)

		got := uc.analyzeImage(ctx, entities.MeterPhotoKindGlobal, bytes, "image/jpeg")

		So(got.Values, ShouldResemble, []float64{1234.567})
		So(got.Confidence, ShouldResemble, []float64{0.92})
	})

	Convey("Detail photo: 3 values, confidence replicated across 3 slots", t, func() {
		r := &mockMeterReader{}
		r.On("ReadMeterPhoto", ctx, entities.MeterPhotoKindDetail, bytes, "image/png").
			Return([]float64{50.123, 200.500, 300.250}, 0.75, nil)
		uc := ucWithReader(r)

		got := uc.analyzeImage(ctx, entities.MeterPhotoKindDetail, bytes, "image/png")

		So(got.Values, ShouldResemble, []float64{50.123, 200.500, 300.250})
		So(got.Confidence, ShouldResemble, []float64{0.75, 0.75, 0.75})
	})

	Convey("Reader error → empty suggestion (graceful fallback to manual entry)", t, func() {
		r := &mockMeterReader{}
		r.On("ReadMeterPhoto", ctx, entities.MeterPhotoKindGlobal, bytes, "image/jpeg").
			Return(nil, 0.0, errors.New("vertex ai blew up"))
		uc := ucWithReader(r)

		got := uc.analyzeImage(ctx, entities.MeterPhotoKindGlobal, bytes, "image/jpeg")

		So(got.Values, ShouldBeNil)
		So(got.Confidence, ShouldBeNil)
	})

	Convey("Empty values → empty suggestion (no detection)", t, func() {
		r := &mockMeterReader{}
		r.On("ReadMeterPhoto", ctx, entities.MeterPhotoKindGlobal, bytes, "image/jpeg").
			Return([]float64{}, 0.0, nil)
		uc := ucWithReader(r)

		got := uc.analyzeImage(ctx, entities.MeterPhotoKindGlobal, bytes, "image/jpeg")

		So(got.Values, ShouldBeNil)
		So(got.Confidence, ShouldBeNil)
	})
}

// TestPhotoMimeType verifies the helper that feeds Gemini the right
// inline-data hint: persisted Content-Type wins, image/jpeg as default.
func TestPhotoMimeType(t *testing.T) {
	Convey("Global photo with persisted content type returns it", t, func() {
		m := entities.MeterReading{GlobalPhotoContentType: "image/png"}
		So(photoMimeType(m, entities.MeterPhotoKindGlobal), ShouldEqual, "image/png")
	})

	Convey("Detail photo with persisted content type returns it", t, func() {
		m := entities.MeterReading{DetailPhotoContentType: "image/heic"}
		So(photoMimeType(m, entities.MeterPhotoKindDetail), ShouldEqual, "image/heic")
	})

	Convey("Missing content type defaults to image/jpeg", t, func() {
		m := entities.MeterReading{}
		So(photoMimeType(m, entities.MeterPhotoKindGlobal), ShouldEqual, "image/jpeg")
		So(photoMimeType(m, entities.MeterPhotoKindDetail), ShouldEqual, "image/jpeg")
	})

	Convey("Unknown kind defaults to image/jpeg", t, func() {
		m := entities.MeterReading{GlobalPhotoContentType: "image/png"}
		So(photoMimeType(m, entities.MeterPhotoKind("bogus")), ShouldEqual, "image/jpeg")
	})
}
