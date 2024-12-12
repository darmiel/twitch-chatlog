package main

func CompareArrays(previous, next []*ListeningChannel) (join []*ListeningChannel, leave []*ListeningChannel) {
	for _, n := range next {
		found := false
		for _, p := range previous {
			if n.ChannelName == p.ChannelName {
				found = true
				break
			}
		}
		if !found {
			join = append(join, n)
		}
	}
	for _, p := range previous {
		found := false
		for _, n := range next {
			if n.ChannelName == p.ChannelName {
				found = true
				break
			}
		}
		if !found {
			leave = append(leave, p)
		}
	}
	return
}

func strOrNil(inp string) *string {
	if inp == "" {
		return nil
	}
	return &inp
}
