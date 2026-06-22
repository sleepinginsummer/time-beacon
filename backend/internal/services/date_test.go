package services

import ("testing"; "time")
func d(s string) time.Time { v,_:=time.Parse("2006-01-02",s); return v }
func TestCalcEndDate(t *testing.T){
 cases:=[]struct{start,typ string; val int; want string}{
  {"2026-01-01","day",30,"2026-01-31"},
  {"2026-01-31","month",1,"2026-02-28"},
  {"2028-02-29","year",1,"2029-02-28"},
 }
 for _,c:=range cases{ if got:=CalcEndDate(d(c.start),c.typ,c.val).Format("2006-01-02"); got!=c.want{ t.Fatalf("%v got %s want %s",c,got,c.want)} }
}
