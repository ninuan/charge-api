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
