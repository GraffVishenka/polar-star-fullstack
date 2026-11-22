package auth

import "time"

// UserDTO — публичное представление пользователя в API. Никогда не содержит
// password_hash.
type UserDTO struct {
	ID                 int64     `json:"id"`
	Login              string    `json:"login"`
	FullName           string    `json:"full_name"`
	Role               Role      `json:"role"`
	BuildingID         *int64    `json:"building_id"`
	BuildingName       *string   `json:"building_name"`
	IsActive           bool      `json:"is_active"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// NewUserDTO конвертирует доменную модель в DTO.
func NewUserDTO(u *User) UserDTO {
	if u == nil {
		return UserDTO{}
	}
	return UserDTO{
		ID:                 u.ID,
		Login:              u.Login,
		FullName:           u.FullName,
		Role:               u.Role,
		BuildingID:         u.BuildingID,
		BuildingName:       u.BuildingName,
		IsActive:           u.IsActive,
		MustChangePassword: u.MustChangePassword,
		CreatedAt:          u.CreatedAt,
		UpdatedAt:          u.UpdatedAt,
	}
}

// SessionDTO — ответ login/refresh (AUTH-2). Refresh-токен передаётся только
// httpOnly-cookie и в теле не появляется.
type SessionDTO struct {
	AccessToken string  `json:"access_token"`
	TokenType   string  `json:"token_type"`
	ExpiresIn   int     `json:"expires_in"`
	User        UserDTO `json:"user"`
}

// NewSessionDTO формирует ответ сессии.
func NewSessionDTO(session *Session) SessionDTO {
	return SessionDTO{
		AccessToken: session.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   session.AccessExpiresIn,
		User:        NewUserDTO(session.User),
	}
}
