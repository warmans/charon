package main

import (
	"github.com/piotrkowalczuk/charon"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type getPermissionHandler struct {
	*handler
}

func (gph *getPermissionHandler) handle(ctx context.Context, req *charon.GetPermissionRequest) (*charon.GetPermissionResponse, error) {
	gph.loggerWith("permission_id", req.Id)

	if req.Id < 1 {
		return nil, grpc.Errorf(codes.InvalidArgument, "permission id needs to be greater than zero")
	}

	act, err := gph.retrieveActor(ctx)
	if err != nil {
		return nil, err
	}
	if err = gph.firewall(req, act); err != nil {
		return nil, err
	}

	permission, err := gph.repository.permission.FindOneByID(req.Id)
	if err != nil {
		return nil, err
	}

	return &charon.GetPermissionResponse{
		Permission: permission.Permission().String(),
	}, nil
}

func (gph *getPermissionHandler) firewall(req *charon.GetPermissionRequest, act *actor) error {
	if act.user.IsSuperuser {
		return nil
	}
	if act.permissions.Contains(charon.PermissionCanRetrieve) {
		return nil
	}

	return grpc.Errorf(codes.PermissionDenied, "charond: permission cannot be retrieved, missing permission")
}
