// Package gateway はAPI Gatewayサービスの内部実装を提供する。
//
// OAuth2認証（GitHub/Google）、JWT発行、リクエストルーティングを担当する。
// 外部からアクセス可能な唯一のサービスであり、セキュリティの境界線として
// 機能する。認証済みリクエストにJWTを付与し、内部サービスに転送する。
package gateway
