package jobs

import "deploycrate-ce/email"

type SendMarketingEmailArgs struct {
	Data email.MarketingData
}

func (SendMarketingEmailArgs) Kind() string { return "send_marketing_email" }
