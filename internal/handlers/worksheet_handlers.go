package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/apierrors"
	"github.com/Its-Ameekh/school_software_backend/internal/middleware"
	"github.com/Its-Ameekh/school_software_backend/internal/models"
	"github.com/Its-Ameekh/school_software_backend/internal/services"
)

// WorksheetHandlers implements the three worksheet asset endpoints:
// POST /worksheets/upload-url, POST /worksheets, DELETE /worksheets/:id.
type WorksheetHandlers struct {
	db            *gorm.DB
	uploadService *services.PresignedUploadService
}

// NewWorksheetHandlers matches the call site in main.go:
// handlers.NewWorksheetHandlers(container.DB, uploadService).
func NewWorksheetHandlers(db *gorm.DB, uploadService *services.PresignedUploadService) *WorksheetHandlers {
	return &WorksheetHandlers{db: db, uploadService: uploadService}
}

// UploadURL issues a presigned R2 PUT URL for a worksheet, after checking
// the actor is authorized for the target class and the requested asset
// passes type/size validation. No object is created in R2 by this call.
//
// @Summary      Get a presigned upload URL for a worksheet
// @Tags         worksheets
// @Security     ApiKeyAuth
// @Accept       json
// @Produce      json
// @Param        body body models.UploadURLRequest true "Upload request"
// @Success      200 {object} models.UploadURLResponse
// @Failure      400 {object} apierrors.ErrorResponse
// @Failure      403 {object} apierrors.ErrorResponse
// @Failure      404 {object} apierrors.ErrorResponse
// @Router       /worksheets/upload-url [post]
func (h *WorksheetHandlers) UploadURL(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}
	role, _ := middleware.GetUserRole(c)

	var req models.UploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	info, err := services.GetClassOwnershipInfo(h.db, req.ClassID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apierrors.NotFound(c, "class")
			return
		}
		apierrors.Internal(c, err)
		return
	}
	if !services.IsAuthorizedForClass(info, actorID, role) {
		apierrors.Forbidden(c)
		return
	}

	if err := h.uploadService.ValidateAssetUpload(services.AssetTypeWorksheet, req.ContentType, req.FileSizeBytes); err != nil {
		apierrors.BadRequest(c, err.Error())
		return
	}

	key := h.uploadService.BuildObjectKey(services.AssetTypeWorksheet, req.ClassID, req.Filename)

	presignedURL, expiresAt, err := h.uploadService.GeneratePresignedPutURL(c.Request.Context(), key, req.ContentType, req.FileSizeBytes)
	if err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, models.UploadURLResponse{
		UploadURL:  presignedURL,
		StorageKey: key,
		ExpiresAt:  expiresAt,
	})
}

// ConfirmUpload persists metadata for a worksheet that has already been
// PUT to R2. It re-validates the asset, cross-checks that storage_key's
// embedded class scope matches the request's class_id, rejects a
// storage_key that's already been confirmed, and verifies the object
// actually exists in R2 before writing a row.
//
// @Summary      Confirm a worksheet upload
// @Tags         worksheets
// @Security     ApiKeyAuth
// @Accept       json
// @Produce      json
// @Param        body body models.ConfirmWorksheetRequest true "Confirm request"
// @Success      201 {object} models.Worksheet
// @Failure      400 {object} apierrors.ErrorResponse
// @Failure      403 {object} apierrors.ErrorResponse
// @Failure      404 {object} apierrors.ErrorResponse
// @Router       /worksheets [post]
func (h *WorksheetHandlers) ConfirmUpload(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}
	role, _ := middleware.GetUserRole(c)

	var req models.ConfirmWorksheetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	info, err := services.GetClassOwnershipInfo(h.db, req.ClassID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apierrors.NotFound(c, "class")
			return
		}
		apierrors.Internal(c, err)
		return
	}
	if !services.IsAuthorizedForClass(info, actorID, role) {
		apierrors.Forbidden(c)
		return
	}

	if err := h.uploadService.ValidateAssetUpload(services.AssetTypeWorksheet, req.ContentType, req.FileSizeBytes); err != nil {
		apierrors.BadRequest(c, err.Error())
		return
	}

	// Closes open item #3: storage_key's embedded class segment must
	// match the class_id in the confirm body.
	embeddedClassID, err := services.ExtractClassIDFromKey(req.StorageKey)
	if err != nil || embeddedClassID != req.ClassID {
		apierrors.BadRequest(c, "storage_key does not belong to the specified class")
		return
	}

	// Reject a second confirm of the same storage_key. This is a
	// check-then-insert and is race-prone under concurrent requests — add
	// a DB unique constraint on storage_key (and file_url) via a Goose
	// migration to close that race at the DB layer; see INTEGRATION.md.
	var existing models.Worksheet
	err = h.db.Where("storage_key = ?", req.StorageKey).Take(&existing).Error
	if err == nil {
		apierrors.BadRequest(c, "this storage_key has already been confirmed")
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		apierrors.Internal(c, err)
		return
	}

	// Closes open item #2: verify the object was actually uploaded before
	// persisting a pointer to it.
	exists, err := h.uploadService.ObjectExists(c.Request.Context(), req.StorageKey)
	if err != nil {
		apierrors.Internal(c, err)
		return
	}
	if !exists {
		apierrors.BadRequest(c, "no uploaded object found for this storage_key")
		return
	}

	worksheet := models.Worksheet{
		ClassID:       req.ClassID,
		Title:         req.Title,
		StorageKey:    req.StorageKey,
		FileURL:       h.uploadService.PublicFileURL(req.StorageKey),
		ContentType:   req.ContentType,
		FileSizeBytes: req.FileSizeBytes,
		UploadedBy:    actorID,
	}

	if err := h.db.Create(&worksheet).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.JSON(http.StatusCreated, worksheet)
}

// Delete hard-deletes a worksheet: the R2 object is removed first
// (synchronous), then the DB row — deliberately in that order, so a
// failed R2 delete never leaves an orphaned DB pointer.
//
// @Summary      Delete a worksheet
// @Tags         worksheets
// @Security     ApiKeyAuth
// @Param        id path int true "Worksheet ID"
// @Success      204
// @Failure      403 {object} apierrors.ErrorResponse
// @Failure      404 {object} apierrors.ErrorResponse
// @Router       /worksheets/{id} [delete]
func (h *WorksheetHandlers) Delete(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}
	role, _ := middleware.GetUserRole(c)

	id := c.Param("id")

	var worksheet models.Worksheet
	if err := h.db.Where("id = ?", id).Take(&worksheet).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apierrors.NotFound(c, "worksheet")
			return
		}
		apierrors.Internal(c, err)
		return
	}

	info, err := services.GetClassOwnershipInfo(h.db, worksheet.ClassID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apierrors.NotFound(c, "class")
			return
		}
		apierrors.Internal(c, err)
		return
	}
	if !services.IsAuthorizedForClass(info, actorID, role) {
		apierrors.Forbidden(c)
		return
	}

	if err := h.uploadService.DeleteObject(c.Request.Context(), worksheet.StorageKey); err != nil {
		apierrors.Internal(c, err)
		return
	}

	if err := h.db.Delete(&worksheet).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}