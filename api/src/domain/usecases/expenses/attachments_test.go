package expenses

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// ─── RequestUploadURL ──────────────────────────────────────────────

func TestRequestUploadURL(t *testing.T) {
	Convey("Given an existing expense and an authorized actor", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, stor, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1"}, nil)
		attsStore.On("Count", ctx, "e1").Return(0, nil)
		stor.On("SignedPutURL",
			ctx,
			mock.MatchedBy(func(name string) bool {
				return len(name) > len("expenses/e1/") && name[:len("expenses/e1/")] == "expenses/e1/"
			}),
			"image/jpeg",
			int64(1234),
			mock.AnythingOfType("time.Duration"),
		).Return("https://signed.example/put", nil)

		Convey("Returns a signed URL, a stable object name, and the bound content-type", func() {
			out, err := uc.RequestUploadURL(ctx, "e1", RequestUploadInput{
				ActorUserID:      "uid-rdc",
				OriginalFilename: "ticket.jpg",
				ContentType:      "image/jpeg",
				SizeBytes:        1234,
			})
			So(err, ShouldBeNil)
			So(out.AttachmentID, ShouldNotBeBlank)
			So(out.ObjectName, ShouldStartWith, "expenses/e1/")
			So(out.ObjectName, ShouldEndWith, ".jpg")
			So(out.UploadURL, ShouldEqual, "https://signed.example/put")
			So(out.ContentType, ShouldEqual, "image/jpeg")
		})
	})

	Convey("Rejects an unsupported MIME type", t, func() {
		ctx := context.Background()
		uc, _, _, _, _, _, _ := newUC()
		_, err := uc.RequestUploadURL(ctx, "e1", RequestUploadInput{
			ActorUserID: "uid-rdc",
			ContentType: "application/zip",
			SizeBytes:   100,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects oversized files (>10MB)", t, func() {
		ctx := context.Background()
		uc, _, _, _, _, _, _ := newUC()
		_, err := uc.RequestUploadURL(ctx, "e1", RequestUploadInput{
			ActorUserID: "uid-rdc",
			ContentType: "image/jpeg",
			SizeBytes:   entities.AttachmentMaxSizeBytes + 1,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("Rejects an intruder actor (auth check before resource lookup)", t, func() {
		ctx := context.Background()
		uc, _, foyStore, _, _, _, _ := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		_, err := uc.RequestUploadURL(ctx, "e1", RequestUploadInput{
			ActorUserID: "intruder",
			ContentType: "image/jpeg",
			SizeBytes:   100,
		})
		So(errors.Is(err, entities.AuthorizationError{}), ShouldBeTrue)
	})

	Convey("Returns 404 for a missing expense", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, _, _ := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "ghost").Return((*entities.Expense)(nil), nil)
		_, err := uc.RequestUploadURL(ctx, "ghost", RequestUploadInput{
			ActorUserID: "uid-rdc",
			ContentType: "image/jpeg",
			SizeBytes:   100,
		})
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})

	Convey("Rejects when the expense already has 10 attachments", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, _, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1"}, nil)
		attsStore.On("Count", ctx, "e1").Return(entities.AttachmentMaxPerExpense, nil)
		_, err := uc.RequestUploadURL(ctx, "e1", RequestUploadInput{
			ActorUserID: "uid-rdc",
			ContentType: "image/jpeg",
			SizeBytes:   100,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})
}

// ─── RecordAttachment ──────────────────────────────────────────────

func TestRecordAttachment(t *testing.T) {
	Convey("Given a successful upload (HEAD matches declaration)", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, stor, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1"}, nil)
		stor.On("Head", ctx, "expenses/e1/att1.pdf").Return(
			interfaces.ObjectStat{SizeBytes: 4242, ContentType: "application/pdf"}, true, nil,
		)
		attsStore.On("CreateIfUnderCap", ctx, "e1", mock.AnythingOfType("entities.Attachment"), entities.AttachmentMaxPerExpense).Return(nil)

		att, err := uc.RecordAttachment(ctx, "e1", RecordAttachmentInput{
			ActorUserID:      "uid-rdc",
			AttachmentID:     "att1",
			ContentType:      "application/pdf",
			SizeBytes:        4242,
			OriginalFilename: "edf.pdf",
		})

		Convey("It records the metadata using GCS-verified values and returns the attachment", func() {
			So(err, ShouldBeNil)
			So(att.ID, ShouldEqual, "att1")
			So(att.ObjectName, ShouldEqual, "expenses/e1/att1.pdf")
			So(att.UploadedBy, ShouldEqual, "uid-rdc")
			// Persisted size/content-type come from HEAD, not the client
			// declaration.
			So(att.SizeBytes, ShouldEqual, int64(4242))
			So(att.ContentType, ShouldEqual, "application/pdf")
		})
	})

	Convey("When HEAD reports a size mismatch, the orphan blob is cleaned up", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, stor, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1"}, nil)
		stor.On("Head", ctx, "expenses/e1/att1.pdf").Return(
			interfaces.ObjectStat{SizeBytes: 9999, ContentType: "application/pdf"}, true, nil,
		)
		stor.On("Delete", ctx, "expenses/e1/att1.pdf").Return(nil)

		_, err := uc.RecordAttachment(ctx, "e1", RecordAttachmentInput{
			ActorUserID:  "uid-rdc",
			AttachmentID: "att1",
			ContentType:  "application/pdf",
			SizeBytes:    4242,
		})

		Convey("The validation error fires and no metadata is written", func() {
			So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
			attsStore.AssertNotCalled(t, "CreateIfUnderCap", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			stor.AssertCalled(t, "Delete", ctx, "expenses/e1/att1.pdf")
		})
	})

	Convey("When HEAD returns an empty content-type, the orphan blob is cleaned up", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, stor, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1"}, nil)
		stor.On("Head", ctx, "expenses/e1/att1.pdf").Return(
			interfaces.ObjectStat{SizeBytes: 4242, ContentType: ""}, true, nil,
		)
		stor.On("Delete", ctx, "expenses/e1/att1.pdf").Return(nil)

		_, err := uc.RecordAttachment(ctx, "e1", RecordAttachmentInput{
			ActorUserID:  "uid-rdc",
			AttachmentID: "att1",
			ContentType:  "application/pdf",
			SizeBytes:    4242,
		})

		Convey("Validation error fires (empty content-type would silently bypass the type check)", func() {
			So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
			attsStore.AssertNotCalled(t, "CreateIfUnderCap", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			stor.AssertCalled(t, "Delete", ctx, "expenses/e1/att1.pdf")
		})
	})

	Convey("When the object isn't there at all, returns a validation error", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, stor, _ := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1"}, nil)
		stor.On("Head", ctx, "expenses/e1/att1.jpg").Return(interfaces.ObjectStat{}, false, nil)

		_, err := uc.RecordAttachment(ctx, "e1", RecordAttachmentInput{
			ActorUserID:  "uid-rdc",
			AttachmentID: "att1",
			ContentType:  "image/jpeg",
			SizeBytes:    100,
		})
		So(errors.Is(err, entities.ValidationError{}), ShouldBeTrue)
	})

	Convey("When the cap is reached at commit time, the blob is rolled back", t, func() {
		ctx := context.Background()
		uc, expStore, foyStore, _, _, stor, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		expStore.On("FindByID", ctx, "e1").Return(&entities.Expense{ID: "e1"}, nil)
		stor.On("Head", ctx, "expenses/e1/att1.jpg").Return(
			interfaces.ObjectStat{SizeBytes: 100, ContentType: "image/jpeg"}, true, nil,
		)
		// Race-loser path: another concurrent upload won the transaction.
		attsStore.On("CreateIfUnderCap", ctx, "e1", mock.Anything, entities.AttachmentMaxPerExpense).
			Return(domainerrors.ErrAlreadyExists)
		stor.On("Delete", ctx, "expenses/e1/att1.jpg").Return(nil)

		_, err := uc.RecordAttachment(ctx, "e1", RecordAttachmentInput{
			ActorUserID:  "uid-rdc",
			AttachmentID: "att1",
			ContentType:  "image/jpeg",
			SizeBytes:    100,
		})
		So(errors.Is(err, domainerrors.ErrAlreadyExists), ShouldBeTrue)
		stor.AssertCalled(t, "Delete", ctx, "expenses/e1/att1.jpg")
	})
}

// ─── GetDownloadURL ────────────────────────────────────────────────

func TestGetDownloadURL(t *testing.T) {
	Convey("Returns a signed URL when the attachment exists", t, func() {
		ctx := context.Background()
		uc, _, foyStore, _, _, stor, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		attsStore.On("FindByID", ctx, "e1", "att1").Return(
			&entities.Attachment{ID: "att1", ObjectName: "expenses/e1/att1.pdf"}, nil,
		)
		stor.On("SignedGetURL", ctx, "expenses/e1/att1.pdf", mock.AnythingOfType("time.Duration")).Return("https://signed.example/get", nil)

		url, _, err := uc.GetDownloadURL(ctx, "e1", "att1", "uid-rdc")
		So(err, ShouldBeNil)
		So(url, ShouldEqual, "https://signed.example/get")
	})

	Convey("404s when the attachment isn't on the expense", t, func() {
		ctx := context.Background()
		uc, _, foyStore, _, _, _, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		attsStore.On("FindByID", ctx, "e1", "ghost").Return((*entities.Attachment)(nil), nil)

		_, _, err := uc.GetDownloadURL(ctx, "e1", "ghost", "uid-rdc")
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}

// ─── DeleteAttachment ──────────────────────────────────────────────

func TestDeleteAttachment(t *testing.T) {
	Convey("Drops both the GCS blob and the metadata", t, func() {
		ctx := context.Background()
		uc, _, foyStore, _, _, stor, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		attsStore.On("FindByID", ctx, "e1", "att1").Return(
			&entities.Attachment{ID: "att1", ObjectName: "expenses/e1/att1.png"}, nil,
		)
		stor.On("Delete", ctx, "expenses/e1/att1.png").Return(nil)
		attsStore.On("Delete", ctx, "e1", "att1").Return(nil)

		err := uc.DeleteAttachment(ctx, "e1", "att1", "uid-rdc")
		So(err, ShouldBeNil)
		stor.AssertCalled(t, "Delete", ctx, "expenses/e1/att1.png")
		attsStore.AssertCalled(t, "Delete", ctx, "e1", "att1")
	})

	Convey("404s when the attachment doesn't exist", t, func() {
		ctx := context.Background()
		uc, _, foyStore, _, _, _, attsStore := newUC()
		foyStore.On("FindByFloor", ctx, entities.FoyerFloorRDC).Return(rdc, nil)
		foyStore.On("FindByFloor", ctx, entities.FoyerFloor1er).Return(one, nil)
		attsStore.On("FindByID", ctx, "e1", "ghost").Return((*entities.Attachment)(nil), nil)

		err := uc.DeleteAttachment(ctx, "e1", "ghost", "uid-rdc")
		So(errors.Is(err, domainerrors.ErrNotFound), ShouldBeTrue)
	})
}
