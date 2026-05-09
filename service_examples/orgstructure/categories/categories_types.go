package categories

type Category struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type CreateCategoryRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type UpdateCategoryRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type ListCategoriesResponse struct {
	Categories []Category `json:"categories"`
}

type GetCategoryResponse struct {
	Category Category `json:"category"`
}
type DeleteCategoryResponse struct {
	Message string `json:"message"`
}