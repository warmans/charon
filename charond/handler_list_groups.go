package main

import (
	"github.com/piotrkowalczuk/charon"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type listGroupsHandler struct {
	*handler
}

func (lgh *listGroupsHandler) handle(ctx context.Context, req *charon.ListGroupsRequest) (*charon.ListGroupsResponse, error) {
	act, err := lgh.retrieveActor(ctx)
	if err != nil {
		return nil, err
	}
	if err = lgh.firewall(req, act); err != nil {
		return nil, err
	}

	entities, err := lgh.repository.group.Find(&groupCriteria{
		offset: req.Offset.Int64Or(0),
		limit:  req.Limit.Int64Or(10),
	})
	if err != nil {
		return nil, err
	}

	groups := make([]*charon.Group, 0, len(entities))
	for _, e := range entities {
		groups = append(groups, e.Message())
	}
	return &charon.ListGroupsResponse{
		Groups: groups,
	}, nil
}

func (lgh *listGroupsHandler) firewall(req *charon.ListGroupsRequest, act *actor) error {
	if act.user.IsSuperuser {
		return nil
	}
	if act.permissions.Contains(charon.GroupCanRetrieve) {
		return nil
	}

	return grpc.Errorf(codes.PermissionDenied, "charond: list of groups cannot be retrieved, missing permission")
}
