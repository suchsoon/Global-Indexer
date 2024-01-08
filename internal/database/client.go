package database

import (
	"context"
	"database/sql"
	"github.com/ethereum/go-ethereum/common"
	"github.com/naturalselectionlabs/global-indexer/schema"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

type Client interface {
	Session
	Transaction

	// FindNodes returns a list of nodes
	FindNodes(ctx context.Context, nodeAddresses []common.Address) ([]*schema.Node, error)
}

type Session interface {
	Migrate(ctx context.Context) error
	WithTransaction(ctx context.Context, transactionFunction func(ctx context.Context, client Client) error, transactionOptions ...*sql.TxOptions) error
	Begin(ctx context.Context, transactionOptions ...*sql.TxOptions) (Client, error)
}

type Transaction interface {
	Rollback() error
	Commit() error
}

var _ goose.Logger = (*SugaredLogger)(nil)

type SugaredLogger struct {
	Logger *zap.SugaredLogger
}

func (s SugaredLogger) Fatalf(format string, v ...interface{}) {
	s.Logger.Fatalf(format, v...)
}

func (s SugaredLogger) Printf(format string, v ...interface{}) {
	s.Logger.Infof(format, v...)
}
