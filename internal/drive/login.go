package drive

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
	"golang.org/x/sync/singleflight"
)

const loginLifetime = 10 * time.Minute

var loginPollWindow = 30 * time.Second

// qrLogin owns the QR login state machine. Its session is guarded by drive.mu;
// replacing or accepting a session also holds drive.commit so disconnects and
// credential writes cannot race with a new login.
type qrLogin struct {
	drive   *Drive
	session *loginSession
}

// LoginSession represents an active QR code login attempt.
type LoginSession struct {
	ID     string `json:"id"`
	QRCode string `json:"qr_code"`
}

// LoginStatus represents the current state of a QR login session.
type LoginStatus struct {
	State pan.LoginState `json:"state"`
}

type loginSession struct {
	id        string
	login     *pan.Login
	state     pan.LoginState
	err       error
	startedAt time.Time
	work      singleflight.Group
}

func (d *Drive) BeginLogin(ctx context.Context) (LoginSession, error) {
	return d.login.begin(ctx)
}

func (d *Drive) LoginStatus(ctx context.Context, id string) (LoginStatus, error) {
	return d.login.status(ctx, id)
}

func (l *qrLogin) begin(ctx context.Context) (LoginSession, error) {
	d := l.drive
	if err := d.commit.Lock(ctx); err != nil {
		return LoginSession{}, err
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		d.commit.Unlock()
		return LoginSession{}, context.Canceled
	}
	session := &loginSession{id: uuid.NewString(), state: pan.LoginWaiting, startedAt: time.Now()}
	l.session = session
	d.mu.Unlock()
	d.commit.Unlock()

	login, err := d.client.BeginLogin(ctx)
	d.mu.Lock()
	defer d.mu.Unlock()
	if l.session != session || d.closed {
		return LoginSession{}, domain.E(domain.KindConflict, "115 登录会话已变更，请重新扫码", nil)
	}
	if err != nil {
		l.session = nil
		return LoginSession{}, fmt.Errorf("start 115 login: %w", err)
	}
	session.login = login
	return LoginSession{
		ID: session.id, QRCode: "data:image/png;base64," + base64.StdEncoding.EncodeToString(login.QRCode),
	}, nil
}

func (l *qrLogin) status(ctx context.Context, id string) (LoginStatus, error) {
	d := l.drive
	if err := ctx.Err(); err != nil {
		return LoginStatus{}, err
	}
	d.mu.Lock()
	session := l.session
	if session == nil || session.id != id || session.login == nil && session.err == nil && session.state == pan.LoginWaiting {
		d.mu.Unlock()
		return LoginStatus{State: pan.LoginExpired}, nil
	}
	d.mu.Unlock()
	result := session.work.DoChan("status", func() (any, error) {
		done, ok := d.StartWork()
		if !ok {
			return LoginStatus{}, context.Canceled
		}
		defer done()
		return l.poll(ctx, session)
	})
	select {
	case <-ctx.Done():
		return LoginStatus{}, ctx.Err()
	case completed := <-result:
		if completed.Err != nil {
			return LoginStatus{}, completed.Err
		}
		return completed.Val.(LoginStatus), nil
	}
}

func (l *qrLogin) poll(ctx context.Context, session *loginSession) (LoginStatus, error) {
	d := l.drive
	d.mu.Lock()
	if l.session != session {
		d.mu.Unlock()
		return LoginStatus{State: pan.LoginExpired}, nil
	}
	if session.err != nil || session.state != pan.LoginWaiting && session.state != pan.LoginScanned {
		state, err := session.state, session.err
		d.mu.Unlock()
		return LoginStatus{State: state}, err
	}
	if time.Since(session.startedAt) > loginLifetime {
		session.state = pan.LoginExpired
		session.login = nil
		d.mu.Unlock()
		return LoginStatus{State: pan.LoginExpired}, nil
	}
	login := session.login
	d.mu.Unlock()

	pollContext, stopPoll := context.WithTimeout(context.WithoutCancel(ctx), loginPollWindow)
	state, err := d.client.LoginStatus(pollContext, login)
	elapsed := errors.Is(pollContext.Err(), context.DeadlineExceeded)
	stopPoll()
	if err != nil {
		if !elapsed {
			return LoginStatus{}, fmt.Errorf("poll 115 login: %w", err)
		}
		state, err = pan.LoginWaiting, nil
	}
	if state == pan.LoginAuthorized {
		err = l.complete(ctx, session, login)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if l.session != session {
		return LoginStatus{State: pan.LoginExpired}, nil
	}
	if err != nil {
		session.err = err
		return LoginStatus{}, err
	}
	if state == pan.LoginWaiting && session.state == pan.LoginScanned {
		state = pan.LoginScanned
	}
	session.state = state
	if state != pan.LoginWaiting && state != pan.LoginScanned {
		session.login = nil
	}
	return LoginStatus{State: state}, nil
}

func (l *qrLogin) complete(ctx context.Context, session *loginSession, login *pan.Login) error {
	d := l.drive
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamTimeout)
	defer cancel()
	d.mu.Lock()
	current := l.session == session
	d.mu.Unlock()
	if !current {
		return nil
	}
	tokens, err := d.client.ExchangeToken(ctx, login)
	if err != nil {
		return fmt.Errorf("complete 115 login: %w", err)
	}
	return d.acceptLogin(ctx, session, tokens)
}
