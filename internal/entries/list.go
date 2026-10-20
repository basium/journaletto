package entries

import "context"

// PageSize is the number of entries per list page.
const PageSize = 20

// ListItem is one row of the entry list.
type ListItem struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	JournalDate      string `json:"journal_date"`
	CreatedAt        string `json:"created_at"` // RFC3339 UTC
	Status           string `json:"status"`     // active | deleted
	ProcessingStatus string `json:"processing_status"`
	Deleted          bool   `json:"deleted"`
}

// ListPage is one page of the user's entries, newest first.
type ListPage struct {
	Items          []ListItem `json:"items"`
	Page           int        `json:"page"`
	PageSize       int        `json:"page_size"`
	Total          int        `json:"total"`
	HasPrev        bool       `json:"has_prev"`
	HasNext        bool       `json:"has_next"`
	PrevPage       int        `json:"-"`
	NextPage       int        `json:"-"`
	IncludeDeleted bool       `json:"include_deleted"`
}

// List returns one page (1-based, clamped to >= 1) of the user's entries ordered newest first.
// Soft-deleted entries are excluded unless includeDeleted is true. This is the single filtering
// point for entry listings.
func (s *Service) List(ctx context.Context, includeDeleted bool, page int) (ListPage, error) {
	if page < 1 {
		page = 1
	}
	where := `user_id=?`
	if !includeDeleted {
		where += ` AND status<>'deleted'`
	}
	out := ListPage{Items: []ListItem{}, Page: page, PageSize: PageSize, IncludeDeleted: includeDeleted}
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE `+where, s.UserID).Scan(&out.Total); err != nil {
		return ListPage{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,title,journal_date,created_at,status,processing_status FROM entries
		WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, s.UserID, PageSize, (page-1)*PageSize)
	if err != nil {
		return ListPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var it ListItem
		if err := rows.Scan(&it.ID, &it.Title, &it.JournalDate, &it.CreatedAt, &it.Status, &it.ProcessingStatus); err != nil {
			return ListPage{}, err
		}
		it.Deleted = it.Status == "deleted"
		out.Items = append(out.Items, it)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, err
	}
	out.HasPrev, out.HasNext = page > 1, page*PageSize < out.Total
	out.PrevPage, out.NextPage = page-1, page+1
	return out, nil
}
