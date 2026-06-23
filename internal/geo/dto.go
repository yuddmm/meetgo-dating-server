package geo

import "github.com/google/uuid"

// updateGeoRequest is the body of POST /my-geo.
type updateGeoRequest struct {
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
	Accuracy *float64 `json:"accuracy"` // meters, optional
}

// setLocationRequest is the body of PUT /me/location.
type setLocationRequest struct {
	Mode   string  `json:"mode"`   // AUTO | MANUAL
	CityID *string `json:"cityId"` // required for MANUAL
}

// cityDTO is a city reference for pickers and responses.
type cityDTO struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"` // COALESCE(name_ru, name)
	Region *string   `json:"region,omitempty"`
}

// locationResponse is returned by PUT /me/location.
type locationResponse struct {
	Mode string   `json:"mode"`
	City *cityDTO `json:"city"`
}
