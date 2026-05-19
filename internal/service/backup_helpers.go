package service

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullable(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func nullableID(i *int64) any {
	if i == nil {
		return nil
	}
	return *i
}
