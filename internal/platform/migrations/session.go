package migrations

import (
	"context"
	"database/sql/driver"
	"errors"
)

// pinnedConnector lends a reserved driver connection to a private, single-slot
// sql.DB during sql.Conn.Raw. It never owns or reconnects the underlying session.
type pinnedConnector struct {
	conn driver.Conn
	used bool
}

func (c *pinnedConnector) Connect(context.Context) (driver.Conn, error) {
	if c.used {
		return nil, errors.New("migration session cannot reconnect")
	}
	c.used = true
	return &borrowedConn{Conn: c.conn}, nil
}
func (c *pinnedConnector) Driver() driver.Driver { return pinnedDriver{} }

type pinnedDriver struct{}

func (pinnedDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("migration session requires connector")
}

type borrowedConn struct{ driver.Conn }

func (*borrowedConn) Close() error { return nil }
func (c *borrowedConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if impl, ok := c.Conn.(driver.ExecerContext); ok {
		return impl.ExecContext(ctx, q, args)
	}
	return nil, driver.ErrSkip
}
func (c *borrowedConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if impl, ok := c.Conn.(driver.QueryerContext); ok {
		return impl.QueryContext(ctx, q, args)
	}
	return nil, driver.ErrSkip
}
func (c *borrowedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if impl, ok := c.Conn.(driver.ConnBeginTx); ok {
		return impl.BeginTx(ctx, opts)
	}
	if opts.ReadOnly || opts.Isolation != 0 {
		return nil, errors.New("migration driver does not support transaction options")
	}
	return c.Conn.Begin()
}
func (c *borrowedConn) CheckNamedValue(v *driver.NamedValue) error {
	if impl, ok := c.Conn.(driver.NamedValueChecker); ok {
		return impl.CheckNamedValue(v)
	}
	return driver.ErrSkip
}
