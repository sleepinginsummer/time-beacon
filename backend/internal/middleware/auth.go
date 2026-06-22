package middleware

import (
 "context"; "net/http"; "strings"
 "github.com/golang-jwt/jwt/v5"
)
func Auth(secret string, next http.Handler) http.Handler { return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
 h:=r.Header.Get("Authorization"); if !strings.HasPrefix(h,"Bearer "){ http.Error(w,"未登录",401); return }
 tok,err:=jwt.Parse(strings.TrimPrefix(h,"Bearer "),func(t *jwt.Token)(any,error){return []byte(secret),nil}); if err!=nil||!tok.Valid{http.Error(w,"登录已失效",401);return}
 c:=tok.Claims.(jwt.MapClaims); ctx:=context.WithValue(r.Context(),"uid",int64(c["uid"].(float64))); ctx=context.WithValue(ctx,"role",c["role"].(string)); next.ServeHTTP(w,r.WithContext(ctx)) }) }
func CORS(next http.Handler) http.Handler { return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){ w.Header().Set("Access-Control-Allow-Origin","*"); w.Header().Set("Access-Control-Allow-Headers","Content-Type, Authorization"); w.Header().Set("Access-Control-Allow-Methods","GET,POST,OPTIONS"); if r.Method=="OPTIONS"{return}; next.ServeHTTP(w,r) }) }
