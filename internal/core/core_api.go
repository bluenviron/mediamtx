package core

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func newInternalUserIDs(count int) []uuid.UUID {
	ids := make([]uuid.UUID, count)
	for i := range ids {
		ids[i] = uuid.New()
	}
	return ids
}

func internalUserIndex(ids []uuid.UUID, id uuid.UUID) int {
	for i, existing := range ids {
		if existing == id {
			return i
		}
	}
	return -1
}

type configGlobalPatchReq struct {
	conf conf.OptionalGlobal
	res  chan error
}

type configPathDefaultsPatchReq struct {
	conf conf.OptionalPath
	res  chan error
}

type configPathAddReq struct {
	name string
	conf conf.OptionalPath
	res  chan error
}

type configPathPatchReq struct {
	name string
	conf conf.OptionalPath
	res  chan error
}

type configPathReplaceReq struct {
	name string
	conf conf.OptionalPath
	res  chan error
}

type configPathDeleteReq struct {
	name string
	res  chan error
}

type configInternalUserRes struct {
	id  uuid.UUID
	err error
}

type configInternalUserReq struct {
	id       uuid.UUID
	user     conf.AuthInternalUser
	optional conf.OptionalAuthInternalUser
	res      chan configInternalUserRes
}

func (p *Core) apiConfigSnapshot() *conf.Conf {
	p.confMutex.RLock()
	defer p.confMutex.RUnlock()
	return p.conf
}

func (p *Core) doAPIConfigGlobalPatch(in conf.OptionalGlobal) (*conf.Conf, error) {
	newConf := p.conf.Clone()

	err := newConf.PatchGlobal(&in)
	if err != nil {
		return nil, err
	}

	err = newConf.Validate(nil)
	if err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathDefaultsPatch(in conf.OptionalPath) (*conf.Conf, error) {
	newConf := p.conf.Clone()
	newConf.PatchPathDefaults(&in)

	err := newConf.Validate(nil)
	if err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathAdd(name string, in conf.OptionalPath) (*conf.Conf, error) {
	newConf := p.conf.Clone()

	err := newConf.AddPath(name, &in)
	if err != nil {
		return nil, err
	}

	err = newConf.Validate(nil)
	if err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathPatch(name string, in conf.OptionalPath) (*conf.Conf, error) {
	newConf := p.conf.Clone()

	err := newConf.PatchPath(name, &in)
	if err != nil {
		return nil, err
	}

	err = newConf.Validate(nil)
	if err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathReplace(name string, in conf.OptionalPath) (*conf.Conf, error) {
	newConf := p.conf.Clone()

	err := newConf.ReplacePath(name, &in)
	if err != nil {
		return nil, err
	}

	err = newConf.Validate(nil)
	if err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

func (p *Core) doAPIConfigPathDelete(name string) (*conf.Conf, error) {
	newConf := p.conf.Clone()

	err := newConf.RemovePath(name)
	if err != nil {
		return nil, err
	}

	err = newConf.Validate(nil)
	if err != nil {
		return nil, err
	}

	p.Log(logger.Info, "reloading configuration (API request)")
	return newConf, nil
}

// APIConfigSnapshot implements apiParent.
func (p *Core) APIConfigSnapshot() *conf.Conf {
	return p.apiConfigSnapshot()
}

// APIConfigGlobalPatch implements apiParent.
func (p *Core) APIConfigGlobalPatch(reqCtx context.Context, in conf.OptionalGlobal) error {
	res := make(chan error)
	select {
	case p.chAPIConfigGlobalPatch <- configGlobalPatchReq{conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return reqCtx.Err()
	}
}

// APIConfigPathDefaultsPatch implements apiParent.
func (p *Core) APIConfigPathDefaultsPatch(reqCtx context.Context, in conf.OptionalPath) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathDefaultsPatch <- configPathDefaultsPatchReq{conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return reqCtx.Err()
	}
}

// APIConfigPathsAdd implements apiParent.
func (p *Core) APIConfigPathsAdd(reqCtx context.Context, name string, in conf.OptionalPath) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathAdd <- configPathAddReq{name: name, conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return reqCtx.Err()
	}
}

// APIConfigPathsPatch implements apiParent.
func (p *Core) APIConfigPathsPatch(reqCtx context.Context, name string, in conf.OptionalPath) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathPatch <- configPathPatchReq{name: name, conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return reqCtx.Err()
	}
}

// APIConfigPathsReplace implements apiParent.
func (p *Core) APIConfigPathsReplace(reqCtx context.Context, name string, in conf.OptionalPath) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathReplace <- configPathReplaceReq{name: name, conf: in, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return reqCtx.Err()
	}
}

// APIConfigPathsDelete implements apiParent.
func (p *Core) APIConfigPathsDelete(reqCtx context.Context, name string) error {
	res := make(chan error)
	select {
	case p.chAPIConfigPathDelete <- configPathDeleteReq{name: name, res: res}:
		return <-res
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return reqCtx.Err()
	}
}

// APIConfigInternalUsersSnapshot implements apiParent.
func (p *Core) APIConfigInternalUsersSnapshot() []defs.APIInternalUser {
	p.confMutex.RLock()
	currentConf := p.conf
	currentIDs := p.internalUserIDs
	p.confMutex.RUnlock()

	redacted := conf.Redact(currentConf)

	items := make([]defs.APIInternalUser, len(redacted.AuthInternalUsers))
	for i, user := range redacted.AuthInternalUsers {
		items[i] = defs.APIInternalUser{
			ID:          currentIDs[i],
			Pos:         i + 1,
			User:        user.User,
			Pass:        user.Pass,
			IPs:         user.IPs,
			Permissions: user.Permissions,
		}
	}
	return items
}

func (p *Core) doAPIConfigInternalUserAdd(req configInternalUserReq) (*conf.Conf, []uuid.UUID, uuid.UUID, error) {
	if p.conf.HasDeprecatedCredentials {
		return nil, nil, uuid.Nil, conf.ErrAuthInternalUserDeprecatedCredentials
	}

	newConf := p.conf.Clone()
	newIDs := append([]uuid.UUID(nil), p.internalUserIDs...)

	id := uuid.New()
	newConf.AuthInternalUsers = append(newConf.AuthInternalUsers, req.user)
	newIDs = append(newIDs, id)

	if err := newConf.Validate(nil); err != nil {
		return nil, nil, uuid.Nil, err
	}

	return newConf, newIDs, id, nil
}

func (p *Core) doAPIConfigInternalUserPatch(req configInternalUserReq) (*conf.Conf, []uuid.UUID, uuid.UUID, error) {
	if p.conf.HasDeprecatedCredentials {
		return nil, nil, uuid.Nil, conf.ErrAuthInternalUserDeprecatedCredentials
	}

	newConf := p.conf.Clone()
	newIDs := append([]uuid.UUID(nil), p.internalUserIDs...)

	index := internalUserIndex(newIDs, req.id)
	if index < 0 {
		return nil, nil, uuid.Nil, conf.ErrAuthInternalUserNotFound
	}

	old := newConf.AuthInternalUsers[index]
	if req.optional.User != nil {
		old.User = *req.optional.User
	}
	if req.optional.Pass != nil && *req.optional.Pass != conf.RedactedCredential {
		old.Pass = *req.optional.Pass
	}
	if req.optional.IPs != nil {
		old.IPs = *req.optional.IPs
	}
	if req.optional.Permissions != nil {
		old.Permissions = *req.optional.Permissions
	}
	newConf.AuthInternalUsers[index] = old

	if err := newConf.Validate(nil); err != nil {
		return nil, nil, uuid.Nil, err
	}

	return newConf, newIDs, req.id, nil
}

func (p *Core) doAPIConfigInternalUserReplace(req configInternalUserReq) (*conf.Conf, []uuid.UUID, uuid.UUID, error) {
	if p.conf.HasDeprecatedCredentials {
		return nil, nil, uuid.Nil, conf.ErrAuthInternalUserDeprecatedCredentials
	}

	newConf := p.conf.Clone()
	newIDs := append([]uuid.UUID(nil), p.internalUserIDs...)

	index := internalUserIndex(newIDs, req.id)
	if index < 0 {
		return nil, nil, uuid.Nil, conf.ErrAuthInternalUserNotFound
	}

	if req.user.Pass == conf.RedactedCredential {
		req.user.Pass = newConf.AuthInternalUsers[index].Pass
	}
	newConf.AuthInternalUsers[index] = req.user

	if err := newConf.Validate(nil); err != nil {
		return nil, nil, uuid.Nil, err
	}

	return newConf, newIDs, req.id, nil
}

func (p *Core) doAPIConfigInternalUserDelete(req configInternalUserReq) (*conf.Conf, []uuid.UUID, uuid.UUID, error) {
	if p.conf.HasDeprecatedCredentials {
		return nil, nil, uuid.Nil, conf.ErrAuthInternalUserDeprecatedCredentials
	}

	newConf := p.conf.Clone()
	newIDs := append([]uuid.UUID(nil), p.internalUserIDs...)

	index := internalUserIndex(newIDs, req.id)
	if index < 0 {
		return nil, nil, uuid.Nil, conf.ErrAuthInternalUserNotFound
	}

	newConf.AuthInternalUsers = append(newConf.AuthInternalUsers[:index], newConf.AuthInternalUsers[index+1:]...)
	newIDs = append(newIDs[:index], newIDs[index+1:]...)

	if err := newConf.Validate(nil); err != nil {
		return nil, nil, uuid.Nil, err
	}

	return newConf, newIDs, req.id, nil
}

// APIConfigInternalUsersAdd implements apiParent.
func (p *Core) APIConfigInternalUsersAdd(reqCtx context.Context, user conf.AuthInternalUser) (uuid.UUID, error) {
	req := configInternalUserReq{
		user: user,
		res:  make(chan configInternalUserRes),
	}

	select {
	case p.chAPIConfigInternalUserAdd <- req:
		res := <-req.res
		return res.id, res.err
	case <-p.ctx.Done():
		return uuid.Nil, fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return uuid.Nil, reqCtx.Err()
	}
}

// APIConfigInternalUsersPatch implements apiParent.
func (p *Core) APIConfigInternalUsersPatch(
	reqCtx context.Context,
	id uuid.UUID,
	optional conf.OptionalAuthInternalUser,
) error {
	req := configInternalUserReq{
		id:       id,
		optional: optional,
		res:      make(chan configInternalUserRes),
	}

	select {
	case p.chAPIConfigInternalUserPatch <- req:
		res := <-req.res
		return res.err
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return reqCtx.Err()
	}
}

// APIConfigInternalUsersReplace implements apiParent.
func (p *Core) APIConfigInternalUsersReplace(reqCtx context.Context, id uuid.UUID, user conf.AuthInternalUser) error {
	req := configInternalUserReq{
		id:   id,
		user: user,
		res:  make(chan configInternalUserRes),
	}

	select {
	case p.chAPIConfigInternalUserReplace <- req:
		res := <-req.res
		return res.err
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return reqCtx.Err()
	}
}

// APIConfigInternalUsersDelete implements apiParent.
func (p *Core) APIConfigInternalUsersDelete(reqCtx context.Context, id uuid.UUID) error {
	req := configInternalUserReq{
		id:  id,
		res: make(chan configInternalUserRes),
	}

	select {
	case p.chAPIConfigInternalUserDelete <- req:
		res := <-req.res
		return res.err
	case <-p.ctx.Done():
		return fmt.Errorf("terminated")
	case <-reqCtx.Done():
		return reqCtx.Err()
	}
}
