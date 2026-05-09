package scorm_progress

import (
	"time"

	"github.com/google/uuid"
)

type CourseProgress struct {
	ID          uuid.UUID  `json:"id"`
	CourseID    uuid.UUID  `json:"course_id"`
	EmployeeID  uuid.UUID  `json:"employee_id"`
	Status      string     `json:"status"`
	Score       *int       `json:"score,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	SuspendData *string    `json:"suspend_data,omitempty"`
}

type CourseWithProgress struct {
	ID          uuid.UUID      `json:"id"`
	ClientID    uuid.UUID      `json:"client_id"`
	Title       string         `json:"title"`
	CategoryIDs []uuid.UUID    `json:"category_ids"`
	Description *string        `json:"description,omitempty"`
	Lecturer    *string        `json:"lecturer,omitempty"`
	ScormURL    string         `json:"scorm_url"`
	IsActive    bool           `json:"is_active"`
	Progress    CourseProgress `json:"progress"`
}

type AssignCourseEmployeesRequest struct {
	EmployeeIDs []uuid.UUID `json:"employee_ids"`
	Reassign    bool        `json:"reassign"`
}

type AssignCourseEmployeesResponse struct {
	RequiresReassign           bool        `json:"requires_reassign"`
	AssignedCount              int         `json:"assigned_count"`
	ReassignedCount            int         `json:"reassigned_count"`
	AlreadyAssignedCount       int         `json:"already_assigned_count"`
	AssignedEmployeeIDs        []uuid.UUID `json:"assigned_employee_ids"`
	ReassignedEmployeeIDs      []uuid.UUID `json:"reassigned_employee_ids"`
	AlreadyAssignedEmployeeIDs []uuid.UUID `json:"already_assigned_employee_ids"`
	Message                    string      `json:"message"`
}

type UpdateCourseProgressRequest struct {
	Status      *string    `json:"status,omitempty"`
	Score       *int       `json:"score,omitempty"`
	SuspendData *string    `json:"suspend_data,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type GetCourseProgressResponse struct {
	Progress CourseProgress `json:"progress"`
}

type GetMyCourseProgressResponse struct {
	Progress CourseProgress `json:"progress"`
}

type ListEmployeeCoursesResponse struct {
	Courses []CourseWithProgress `json:"courses"`
}
