package meeting

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

var feedGenders = map[string]bool{"MALE": true, "FEMALE": true}

var feedGoals = map[string]bool{
	"FRIENDSHIP": true, "LONG_TERM_RELATIONSHIP": true, "DATES": true, "NOTHING_SERIOUS": true,
}

// Feed godoc
//
//	@Summary	Discovery feed of nearby ads (keyset cursor)
//	@Tags		meetings
//	@Security	BearerAuth
//	@Produce	json
//	@Param		sort	query		string	false	"distance | date"
//	@Param		radius	query		number	false	"km"
//	@Param		gender	query		string	false	"MALE | FEMALE"
//	@Param		ageMin	query		int		false	"18..99"
//	@Param		ageMax	query		int		false	"18..99"
//	@Param		goal	query		string	false	"dating goal"
//	@Param		tagIds	query		[]string false	"meeting tag ids"
//	@Param		cursor	query		string	false	"opaque cursor"
//	@Param		limit	query		int		false	"page size"
//	@Success	200		{object}	feedResponse
//	@Failure	422		{object}	object
//	@Router		/meeting-list [get]
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	filters, verr := parseFeedFilters(r)
	if verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	resp, err := h.svc.Feed(r.Context(), userID, filters)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func parseFeedFilters(r *http.Request) (feedFilters, *httpx.APIError) {
	q := r.URL.Query()
	details := map[string]string{}
	f := feedFilters{
		Sort:   sortDistance,
		Radius: feedDefaultRadius,
		AgeMin: ageFloor,
		AgeMax: ageCeil,
		Limit:  feedDefaultLimit,
	}

	if v := q.Get("sort"); v != "" {
		if v != sortDistance && v != sortDate {
			details["sort"] = "must be distance or date"
		} else {
			f.Sort = v
		}
	}
	if v := q.Get("radius"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err != nil || n <= 0 || n > feedMaxRadius {
			details["radius"] = "must be 0..500 km"
		} else {
			f.Radius = n
		}
	}
	if v := q.Get("gender"); v != "" {
		if !feedGenders[v] {
			details["gender"] = "must be MALE or FEMALE"
		} else {
			f.Gender = &v
		}
	}
	if v := q.Get("goal"); v != "" {
		if !feedGoals[v] {
			details["goal"] = "invalid value"
		} else {
			f.Goal = &v
		}
	}
	if v := q.Get("ageMin"); v != "" {
		f.AgeMin = parseAge(v, details, "ageMin")
	}
	if v := q.Get("ageMax"); v != "" {
		f.AgeMax = parseAge(v, details, "ageMax")
	}
	if f.AgeMin > f.AgeMax {
		details["ageMin"] = "must be <= ageMax"
	}
	for _, s := range q["tagIds"] {
		id, err := uuid.Parse(s)
		if err != nil {
			details["tagIds"] = "contains an invalid id"
			break
		}
		f.TagIDs = append(f.TagIDs, id)
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err != nil || n < 1 {
			details["limit"] = "must be >= 1"
		} else {
			f.Limit = min(n, feedMaxLimit)
		}
	}
	if v := q.Get("cursor"); v != "" {
		c, cerr := decodeCursor(v, f.Sort)
		if cerr != nil {
			details["cursor"] = "invalid cursor"
		} else {
			f.Cursor = c
		}
	}

	if len(details) > 0 {
		return feedFilters{}, httpx.ValidationError(details)
	}
	return f, nil
}

func parseAge(v string, details map[string]string, field string) int {
	n, err := strconv.Atoi(v)
	if err != nil || n < ageFloor || n > ageCeil {
		details[field] = "must be 18..99"
		return ageFloor
	}
	return n
}
