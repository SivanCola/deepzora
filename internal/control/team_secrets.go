package control

import "context"

func (d teamDirectory) RequestTeamSecret(ctx context.Context, name, purpose, requestID string) (string, error) {
	svc, caller, err := d.caller(ctx)
	if err != nil {
		return "", err
	}
	if err := svc.RequestSecret(ctx, caller, name, purpose, requestID); err != nil {
		return "", teamRefusal(err)
	}
	return "Requested secret authorization. The user enters its value only in the private secret request card. Never ask them to paste it into chat; inspect Team settings for its authorization status.", nil
}
