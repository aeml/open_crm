package main

import (
	"fmt"

	"github.com/aeml/open_crm/apps/api/internal/config"
	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
	moduleemailsequences "github.com/aeml/open_crm/apps/api/internal/modules/emailsequences"
	moduleemailsuppressions "github.com/aeml/open_crm/apps/api/internal/modules/emailsuppressions"
	moduleratelimits "github.com/aeml/open_crm/apps/api/internal/modules/ratelimits"
	modulesequencerunner "github.com/aeml/open_crm/apps/api/internal/modules/sequencerunner"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

func buildSequenceRunner(
	env config.Env,
	hosted bool,
	sequences *moduleemailsequences.Service,
	sender *moduleuseremail.Service,
	messages *moduleemailmessages.Service,
	suppressions *moduleemailsuppressions.Service,
	budgets *moduleratelimits.Service,
) (*modulesequencerunner.Service, error) {
	if !hosted {
		return modulesequencerunner.NewServiceWithSuppressions(sequences, sender, messages, suppressions, env.APIBaseURL), nil
	}
	tenantLimit, senderLimit, err := env.HostedSequenceSendLimits()
	if err != nil {
		return nil, fmt.Errorf("hosted sequence send limits: %w", err)
	}
	return modulesequencerunner.NewServiceWithHostedLimits(
		sequences,
		sender,
		messages,
		suppressions,
		env.APIBaseURL,
		budgets,
		modulesequencerunner.SendLimits{TenantPer24Hours: tenantLimit, SenderPerHour: senderLimit},
	), nil
}
