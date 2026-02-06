package dto

// ChildDto - Thông tin học sinh (con của phụ huynh)
type ChildDto struct {
	ID           string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username     string  `json:"username" example:"student123"`
	FullName     *string `json:"full_name,omitempty" example:"Nguyễn Văn B"`
	AvatarUrl    *string `json:"avatar_url,omitempty" example:"https://example.com/avatar.jpg"`
	Relationship string  `json:"relationship" example:"parent"` // parent, guardian, grandparent
}

// PaginatedChildrenResponse - Response cho GET /profile/children
type PaginatedChildrenResponse struct {
	Children []ChildDto `json:"children"`
	Total    int64      `json:"total" example:"2"`
	Page     int        `json:"page" example:"1"`
	PageSize int        `json:"page_size" example:"20"`
}

// MyOrganizationDto - Thông tin tổ chức của user
type MyOrganizationDto struct {
	ID         string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name       string `json:"name" example:"Trường THPT ABC"`
	MemberRole string `json:"member_role" example:"Admin"` // Role của user trong org
	Status     string `json:"status" example:"active"`
}

// PaginatedOrganizationsResponse - Response cho GET /profile/organizations
type PaginatedOrganizationsResponse struct {
	Organizations []MyOrganizationDto `json:"organizations"`
	Total         int64               `json:"total" example:"1"`
	Page          int                 `json:"page" example:"1"`
	PageSize      int                 `json:"page_size" example:"20"`
}
