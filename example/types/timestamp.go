package types

import (
	"encoding"
	"time"
)

type (
	AliasString  = string
	AliasBool    = bool
	AliasInt     = int
	AliasUint64  = uint64
	AliasFloat64 = float64
	AliasTime    = time.Time

	CustomString  string
	CustomBool    bool
	CustomInt     int
	CustomUint64  uint64
	CustomFloat64 float64
	CustomTime    time.Time
)

var _ interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
} = (*CustomTime)(nil)

func (dt CustomTime) MarshalText() ([]byte, error) {
	return []byte(dt.String()), nil
}

func (dt *CustomTime) UnmarshalText(data []byte) (err error) {
	str := string(data)
	if len(str) == 0 || str == "0" {
		return nil
	}
	*dt, err = ParseTimestampFromString(str)
	return
}

func (dt CustomTime) String() string {
	if dt.IsZero() {
		return ""
	}
	return time.Time(dt).In(CST).Format(time.RFC3339)
}

func ParseTimestampFromString(s string) (dt CustomTime, err error) {
	var t time.Time
	t, err = time.Parse(time.RFC3339, s)
	dt = CustomTime(t)
	return
}

func (dt CustomTime) IsZero() bool {
	unix := dt.Unix()
	return unix == 0 || unix == TimestampZero.Unix()
}

func (dt CustomTime) Unix() int64 {
	return time.Time(dt).Unix()
}

var (
	CST           = time.FixedZone("CST", 8*60*60)
	TimestampZero = CustomTime(time.Time{})
)
