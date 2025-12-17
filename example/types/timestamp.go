package types

import (
	"encoding"
	"time"
)

type Timestamp time.Time

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*Timestamp)(nil)

func (dt Timestamp) MarshalText() ([]byte, error) {
	return []byte(dt.String()), nil
}

func (dt *Timestamp) UnmarshalText(data []byte) (err error) {
	str := string(data)
	if len(str) == 0 || str == "0" {
		return nil
	}
	*dt, err = ParseTimestampFromString(str)
	return
}

func (dt Timestamp) String() string {
	if dt.IsZero() {
		return ""
	}
	return time.Time(dt).In(CST).Format(time.RFC3339)
}

func ParseTimestampFromString(s string) (dt Timestamp, err error) {
	var t time.Time
	t, err = time.Parse(time.RFC3339, s)
	dt = Timestamp(t)
	return
}

func (dt Timestamp) IsZero() bool {
	unix := dt.Unix()
	return unix == 0 || unix == TimestampZero.Unix()
}

func (dt Timestamp) Unix() int64 {
	return time.Time(dt).Unix()
}

var (
	CST           = time.FixedZone("CST", 8*60*60)
	TimestampZero = Timestamp(time.Time{})
)
