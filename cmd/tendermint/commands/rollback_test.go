package commands_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tendermint/tendermint/cmd/tendermint/commands"
	"github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/rpc/client/local"
	rpctest "github.com/tendermint/tendermint/rpc/test"
	e2e "github.com/tendermint/tendermint/test/e2e/app"
)

func TestRollbackIntegration(t *testing.T) {
	var height int64
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg1, err := rpctest.CreateConfig(t, t.Name())
	require.NoError(t, err)
	cfg1.BaseConfig.DBBackend = "goleveldb"

	cfg2, err := rpctest.CreateConfig(t, t.Name()+"Full")
	cfg2.Mode = config.ModeFull
	cfg2.BaseConfig.DBBackend = "goleveldb"

	// connect the peers
	id1, err := cfg1.LoadOrGenNodeKeyID()
	require.NoError(t, err)
	id2, err := cfg2.LoadOrGenNodeKeyID()
	require.NoError(t, err)
	port1 := strings.Split(cfg1.P2P.ListenAddress, ":")[2]
	port2 := strings.Split(cfg2.P2P.ListenAddress, ":")[2]
	cfg1.P2P.PersistentPeers = fmt.Sprintf("tcp://%s@localhost:%s", id2, port2)
	cfg2.P2P.PersistentPeers = fmt.Sprintf("tcp://%s@localhost:%s", id1, port1)
	fmt.Println(cfg1.P2P.PersistentPeers, cfg2.P2P.PersistentPeers)

	app1, err := e2e.NewApplication(e2e.DefaultConfig(dir1))
	require.NoError(t, err)
	app2, err := e2e.NewApplication(e2e.DefaultConfig(dir2))
	require.NoError(t, err)

	t.Run("First run", func(t *testing.T) {
		ctx1, cancel1 := context.WithCancel(ctx)
		defer cancel1()
		ctx2, cancel2 := context.WithCancel(ctx)
		defer cancel2()

		node1, _, err := rpctest.StartTendermint(ctx1, cfg1, app1, rpctest.SuppressStdout)
		require.NoError(t, err)
		node2, _, err := rpctest.StartTendermint(ctx2, cfg2, app2, rpctest.SuppressStdout)
		require.NoError(t, err)

		require.True(t, node1.IsRunning())
		require.True(t, node2.IsRunning())

		time.Sleep(3 * time.Second)

		cancel1()
		node1.Wait()
		cancel2()
		node2.Wait()

		require.False(t, node1.IsRunning())
		require.False(t, node2.IsRunning())
	})
	t.Run("Rollback", func(t *testing.T) {
		time.Sleep(time.Second)
		require.NoError(t, app1.Rollback())
		height, _, err = commands.RollbackState(cfg1)
		require.NoError(t, err, "%d", height)
	})
	t.Run("Rollback again", func(t *testing.T) {
		// should be able to rollback again.
		require.NoError(t, app1.Rollback())
		height2, _, err := commands.RollbackState(cfg1)
		require.NoError(t, err, "%d", height2)
		require.Equal(t, height-1, height2)
	})
	t.Run("Restart", func(t *testing.T) {
		require.True(t, height > 0, "%d", height)

		ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		_, _, err := rpctest.StartTendermint(ctx, cfg2, app2, rpctest.SuppressStdout)
		require.NoError(t, err)
		node1, _, err := rpctest.StartTendermint(ctx, cfg1, app1)
		require.NoError(t, err)

		logger := log.NewNopLogger()

		client, err := local.New(logger, node1.(local.NodeService))
		require.NoError(t, err)

		ticker := time.NewTicker(200 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				t.Fatalf("failed to make progress after 20 seconds. Min height: %d", height)
			case <-ticker.C:
				status, err := client.Status(ctx)
				require.NoError(t, err)

				if status.SyncInfo.LatestBlockHeight > height+2 {
					return
				}
			}
		}
	})

}
