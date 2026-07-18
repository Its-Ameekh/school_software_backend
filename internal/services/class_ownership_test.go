package services

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// NOTE ON THIS FILE'S SQL-STRING MOCK:
// The expected query text below ("SELECT id, teacher_id, ... LIMIT $2")
// is a best guess at what GORM v1.31 + the postgres driver actually emits
// for `.Table(...).Select(...).Where(...).Take(...)`. GORM frequently adds
// its own LIMIT (and sometimes ORDER BY) that isn't obvious from the Go
// code alone. If this test fails against the real generated SQL, capture
// the actual query (run with a real Postgres + `db.Debug()`, or read the
// sqlmock failure diff) and update the ExpectQuery regex here — do NOT
// change GetClassOwnershipInfo's Go code to satisfy the mock.
func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	dialector := postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	})

	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)

	return db, mock
}

func TestGetClassOwnershipInfo_Found(t *testing.T) {
	db, mock := newMockDB(t)

	rows := sqlmock.NewRows([]string{"id", "teacher_id", "substitute_teacher_id", "substitute_active"}).
		AddRow(5, 11, 22, true)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, teacher_id, substitute_teacher_id, substitute_active FROM "classes" WHERE id = $1`,
	)).WillReturnRows(rows)

	info, err := GetClassOwnershipInfo(db, 5)
	require.NoError(t, err)
	assert.Equal(t, uint(5), info.ID)
	require.NotNil(t, info.TeacherID)
	assert.Equal(t, uint(11), *info.TeacherID)
	require.NotNil(t, info.SubstituteTeacherID)
	assert.Equal(t, uint(22), *info.SubstituteTeacherID)
	assert.True(t, info.SubstituteActive)
}

func TestGetClassOwnershipInfo_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, teacher_id, substitute_teacher_id, substitute_active FROM "classes" WHERE id = $1`,
	)).WillReturnRows(sqlmock.NewRows([]string{"id", "teacher_id", "substitute_teacher_id", "substitute_active"}))

	_, err := GetClassOwnershipInfo(db, 999)
	assert.Error(t, err)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestIsAuthorizedForClass(t *testing.T) {
	teacherID := uint(11)
	subID := uint(22)

	activeInfo := &ClassOwnershipInfo{
		ID:                  5,
		TeacherID:           &teacherID,
		SubstituteTeacherID: &subID,
		SubstituteActive:    true,
	}

	assert.True(t, IsAuthorizedForClass(activeInfo, 999, "PRINCIPAL"), "principal always authorized")
	assert.True(t, IsAuthorizedForClass(activeInfo, 11, "TEACHER"), "assigned teacher authorized")
	assert.True(t, IsAuthorizedForClass(activeInfo, 22, "TEACHER"), "active substitute authorized")
	assert.False(t, IsAuthorizedForClass(activeInfo, 33, "TEACHER"), "unrelated teacher not authorized")
	assert.False(t, IsAuthorizedForClass(activeInfo, 11, "PARENT"), "parent role never authorized")

	inactiveInfo := &ClassOwnershipInfo{
		ID:                  5,
		TeacherID:           &teacherID,
		SubstituteTeacherID: &subID,
		SubstituteActive:    false,
	}
	assert.False(t, IsAuthorizedForClass(inactiveInfo, 22, "TEACHER"), "inactive substitute not authorized")

	noTeacherInfo := &ClassOwnershipInfo{ID: 5}
	assert.False(t, IsAuthorizedForClass(noTeacherInfo, 11, "TEACHER"), "nil teacher_id never matches")
}