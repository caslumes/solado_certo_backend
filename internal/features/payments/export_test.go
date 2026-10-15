package payments

import "time"

func (s *service) SetNow(now func() time.Time) {
	s.now = now
}
