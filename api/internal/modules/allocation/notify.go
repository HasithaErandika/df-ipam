package allocation

import (
	"context"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/notifications"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func requestLink(id string) string { return "/requests?id=" + id }

func reservationLink(id string) string { return "/reservations?id=" + id }

func describe(r Request) string {
	what := map[string]string{TypeAllocation: "IP allocation", TypeReservation: "IP reservation", TypeRelease: "IP release"}[r.RequestType]
	target := r.SubnetCIDR
	if r.RequestedAddress != nil {
		target = *r.RequestedAddress
	}
	return fmt.Sprintf("%s request for %s (%s)", what, target, r.SubnetName)
}

func (s *Service) notifyStep(ctx context.Context, tx pgx.Tx, actor string, r Request, step int, role *string) error {
	approvers, err := notifications.Approvers(ctx, tx, role)
	if err != nil {
		return err
	}
	var recipients []string
	for _, a := range approvers {
		if a != r.RequestedBy {
			recipients = append(recipients, a)
		}
	}
	return notifications.Enqueue(ctx, tx, actor, notifications.Event{
		Type: notifications.ApprovalRequested, ObjectType: "ip_request", ObjectID: r.ID,
		DedupeKey: fmt.Sprintf("approval_requested:%s:%d", r.ID, step),
		Title:     "Approval needed: " + describe(r),
		Body:      fmt.Sprintf("%s asked for an %s.\n\nJustification: %s", r.RequestedByEmail, describe(r), r.Justification),
		Link:      requestLink(r.ID), Recipients: recipients,
	})
}

func (s *Service) notifyApproved(ctx context.Context, tx pgx.Tx, p *auth.Principal, r Request) error {
	if err := notifications.Enqueue(ctx, tx, p.UserID, notifications.Event{Type: notifications.RequestApproved,
		ObjectType: "ip_request", ObjectID: r.ID, DedupeKey: "request_approved:" + r.ID,
		Title: "Approved: " + describe(r), Body: fmt.Sprintf("Your %s is approved and waiting to be fulfilled.", describe(r)),
		Link: requestLink(r.ID), Recipients: []string{r.RequestedBy}}); err != nil {
		return err
	}
	perm := auth.IPAllocate
	if r.RequestType == TypeReservation {
		perm = auth.IPReserve
	}
	fulfillers, err := notifications.UsersWithPermission(ctx, tx, perm, nil)
	if err != nil {
		return err
	}
	return notifications.Enqueue(ctx, tx, p.UserID, notifications.Event{Type: notifications.RequestReady,
		ObjectType: "ip_request", ObjectID: r.ID, DedupeKey: "request_ready:" + r.ID,
		Title: "Ready to fulfil: " + describe(r), Body: fmt.Sprintf("The %s from %s is approved and can be fulfilled.", describe(r), r.RequestedByEmail),
		Link: requestLink(r.ID), Recipients: fulfillers})
}

func (s *Service) notifyFulfilled(ctx context.Context, tx pgx.Tx, actor string, r Request, address string) error {
	verb := map[string]string{TypeAllocation: "allocated", TypeReservation: "reserved", TypeRelease: "released"}[r.RequestType]
	return notifications.Enqueue(ctx, tx, actor, notifications.Event{Type: notifications.RequestFulfilled,
		ObjectType: "ip_request", ObjectID: r.ID, DedupeKey: "request_fulfilled:" + r.ID,
		Title: fmt.Sprintf("%s %s", strings.ToUpper(verb[:1])+verb[1:], address),
		Body:  fmt.Sprintf("%s is now %s for your %s.", address, verb, describe(r)),
		Link:  requestLink(r.ID), Recipients: []string{r.RequestedBy}})
}
