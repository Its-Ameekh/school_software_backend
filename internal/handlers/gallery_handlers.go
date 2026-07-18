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

// GalleryHandlers implements the three gallery photo asset endpoints:
// POST /gallery/upload-url, POST /gallery, DELETE /gallery/:id.
// Mirrors WorksheetHandlers exactly, aside from asset-type caps/types and
// the Caption/URL fields.
type GalleryHandlers struct {
	db            *gorm.DB
	uploadService *services.PresignedUploadService
}

// NewGalleryHandlers matches the call site in main.go:
// handlers.NewGalleryHandlers(container.DB, uploadService).
func NewGalleryHandlers(db *gorm.DB, uploadService *services.PresignedUploadService) *GalleryHandlers {
	return &GalleryHandlers{db: db, uploadService: uploadService}
}

// UploadURL issues a presigned R2 PUT URL for a gallery photo.
//
// @Summary      Get a presigned upload URL for a gallery photo
// @Tags         gallery
// @Security     ApiKeyAuth
// @Accept       json
// @Produce      json
// @Param        body body models.UploadURLRequest true "Upload request"
// @Success      200 {object} models.UploadURLResponse
// @Failure      400 {object} apierrors.ErrorResponse
// @Failure      403 {object} apierrors.ErrorResponse
// @Failure      404 {object} apierrors.ErrorResponse
// @Router       /gallery/upload-url [post]
func (h *GalleryHandlers) UploadURL(c *gin.Context) {
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

	if err := h.uploadService.ValidateAssetUpload(services.AssetTypeGallery, req.ContentType, req.FileSizeBytes); err != nil {
		apierrors.BadRequest(c, err.Error())
		return
	}

	key := h.uploadService.BuildObjectKey(services.AssetTypeGallery, req.ClassID, req.Filename)

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

// ConfirmUpload persists metadata for a gallery photo already PUT to R2.
//
// @Summary      Confirm a gallery photo upload
// @Tags         gallery
// @Security     ApiKeyAuth
// @Accept       json
// @Produce      json
// @Param        body body models.ConfirmGalleryRequest true "Confirm request"
// @Success      201 {object} models.GalleryPhoto
// @Failure      400 {object} apierrors.ErrorResponse
// @Failure      403 {object} apierrors.ErrorResponse
// @Failure      404 {object} apierrors.ErrorResponse
// @Router       /gallery [post]
func (h *GalleryHandlers) ConfirmUpload(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}
	role, _ := middleware.GetUserRole(c)

	var req models.ConfirmGalleryRequest
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

	if err := h.uploadService.ValidateAssetUpload(services.AssetTypeGallery, req.ContentType, req.FileSizeBytes); err != nil {
		apierrors.BadRequest(c, err.Error())
		return
	}

	embeddedClassID, err := services.ExtractClassIDFromKey(req.StorageKey)
	if err != nil || embeddedClassID != req.ClassID {
		apierrors.BadRequest(c, "storage_key does not belong to the specified class")
		return
	}

	var existing models.GalleryPhoto
	err = h.db.Where("storage_key = ?", req.StorageKey).Take(&existing).Error
	if err == nil {
		apierrors.BadRequest(c, "this storage_key has already been confirmed")
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		apierrors.Internal(c, err)
		return
	}

	exists, err := h.uploadService.ObjectExists(c.Request.Context(), req.StorageKey)
	if err != nil {
		apierrors.Internal(c, err)
		return
	}
	if !exists {
		apierrors.BadRequest(c, "no uploaded object found for this storage_key")
		return
	}

	photo := models.GalleryPhoto{
		ClassID:       req.ClassID,
		Caption:       req.Caption,
		StorageKey:    req.StorageKey,
		URL:           h.uploadService.PublicFileURL(req.StorageKey),
		ContentType:   req.ContentType,
		FileSizeBytes: req.FileSizeBytes,
		UploadedBy:    actorID,
	}

	if err := h.db.Create(&photo).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.JSON(http.StatusCreated, photo)
}

// Delete hard-deletes a gallery photo: R2 object first, then the DB row.
//
// @Summary      Delete a gallery photo
// @Tags         gallery
// @Security     ApiKeyAuth
// @Param        id path int true "Gallery photo ID"
// @Success      204
// @Failure      403 {object} apierrors.ErrorResponse
// @Failure      404 {object} apierrors.ErrorResponse
// @Router       /gallery/{id} [delete]
func (h *GalleryHandlers) Delete(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}
	role, _ := middleware.GetUserRole(c)

	id := c.Param("id")

	var photo models.GalleryPhoto
	if err := h.db.Where("id = ?", id).Take(&photo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apierrors.NotFound(c, "gallery photo")
			return
		}
		apierrors.Internal(c, err)
		return
	}

	info, err := services.GetClassOwnershipInfo(h.db, photo.ClassID)
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

	if err := h.uploadService.DeleteObject(c.Request.Context(), photo.StorageKey); err != nil {
		apierrors.Internal(c, err)
		return
	}

	if err := h.db.Delete(&photo).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}