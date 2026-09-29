package auth

// LoginReq 登录请求。
type LoginReq struct {
	Username string `json:"username" binding:"required,min=3,max=64" label:"用户名"`
	Password string `json:"password" binding:"required,min=6,max=64" label:"密码"`
}

// RefreshReq 刷新令牌请求。
type RefreshReq struct {
	RefreshToken string `json:"refreshToken" binding:"required" label:"刷新令牌"`
}

// LoginResp 登录响应。
type LoginResp struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
	TokenType    string `json:"tokenType"`
	User         any    `json:"user"`
}
