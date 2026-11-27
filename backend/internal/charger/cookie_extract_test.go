package charger

import "testing"

func TestExtractCookie(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "plain cookie",
			input: "wxopenid=abc123; info=xyz789",
			want:  "wxopenid=abc123; info=xyz789",
		},
		{
			name:  "cookie prefix",
			input: "Cookie: wxopenid=abc123; info=xyz789",
			want:  "wxopenid=abc123; info=xyz789",
		},
		{
			name:  "curl -H single quotes",
			input: "curl 'https://ele.mocele.com/ajax/WxPay/Api/getdevice' \\\n  -H 'Accept: */*' \\\n  -H 'Cookie: deviceid=123; wxopenid=o123; info=tok456' \\\n  --data-raw 'id=123'",
			want:  "deviceid=123; wxopenid=o123; info=tok456",
		},
		{
			name:  "curl -b double quotes",
			input: `curl "https://ele.mocele.com/test" -b "wxopenid=o999; info=tok999"`,
			want:  "wxopenid=o999; info=tok999",
		},
		{
			name:  "raw http headers",
			input: "POST /ajax/test HTTP/1.1\r\nHost: ele.mocele.com\r\nCookie: wxopenid=hdr1; info=hdr2\r\nUser-Agent: MicroMessenger\r\n",
			want:  "wxopenid=hdr1; info=hdr2",
		},
		{
			name:    "multiline without cookie",
			input:   "POST /ajax/test HTTP/1.1\nHost: ele.mocele.com\n",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "   ",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractCookie(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCookieIdentity(t *testing.T) {
	id1 := CookieIdentity("deviceid=123; wxopenid=oABC; info=tok1")
	id2 := CookieIdentity("info=tok2; WXOpenID=oABC; extra=noise; deviceid=999")
	id3 := CookieIdentity("wxopenid=oOTHER; info=tok1")
	if id1 != "wxopenid:oABC" || id1 != id2 {
		t.Fatalf("expected identical wxopenid identity, got id1=%q id2=%q", id1, id2)
	}
	if id1 == id3 {
		t.Fatalf("different wxopenid should not match: %q == %q", id1, id3)
	}

	fb1 := CookieIdentity("b=2; a=1")
	fb2 := CookieIdentity("A=1; b=2")
	if fb1 != "cookie:a=1;b=2" || fb1 != fb2 {
		t.Fatalf("expected canonical sorted fallback, got fb1=%q fb2=%q", fb1, fb2)
	}

	// Duplicate wxopenid must use first-wins (matching upstream), strip quotes,
	// normalize URL encoding, and ignore mutable deviceid in fallback hash.
	dup := CookieIdentity(`wxopenid="oFIRST"; wxopenid=oSPOOFED; info=tok`)
	if dup != "wxopenid:oFIRST" {
		t.Fatalf("duplicate/quoted wxopenid identity = %q, want wxopenid:oFIRST", dup)
	}
	encoded := CookieIdentity("wxopenid=o%41BC")
	if encoded != "wxopenid:oABC" {
		t.Fatalf("url-encoded wxopenid identity = %q, want wxopenid:oABC", encoded)
	}
	noOpenID1 := CookieIdentity("deviceid=111; info=abc")
	noOpenID2 := CookieIdentity("deviceid=999; info=abc")
	if noOpenID1 != noOpenID2 || noOpenID1 != "cookie:info=abc" {
		t.Fatalf("deviceid must be excluded from fallback identity: %q vs %q", noOpenID1, noOpenID2)
	}
	if _, err := ExtractCookie("wxopenid=oREAL; wxopenid=oDUMMY; info=tok"); err == nil {
		t.Fatal("ExtractCookie must reject duplicate wxopenid keys")
	}
}
