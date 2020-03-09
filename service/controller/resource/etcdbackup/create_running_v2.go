package etcdbackup

import (
	"context"
	"fmt"
	"time"

	"github.com/giantswarm/apiextensions/pkg/apis/backup/v1alpha1"
	"github.com/giantswarm/microerror"

	"github.com/giantswarm/etcd-backup-operator/pkg/etcd"
	"github.com/giantswarm/etcd-backup-operator/pkg/giantnetes"
	"github.com/giantswarm/etcd-backup-operator/service/controller/key"
	"github.com/giantswarm/etcd-backup-operator/service/controller/resource/etcdbackup/internal/state"
)

func (r *Resource) backupRunningV2BackupRunningTransition(ctx context.Context, obj interface{}, currentState state.State) (state.State, error) {
	doneSomething, err := r.runBackupOnAllInstances(ctx, obj, r.doV2Backup)
	if err != nil {
		return "", microerror.Mask(err)
	}

	if doneSomething {
		return backupStateRunningV2BackupRunning, nil
	}

	// No work has been done in any of the instances, backup is completed.
	return backupStateRunningV2BackupCompleted, nil
}

func (r *Resource) doV2Backup(ctx context.Context, etcdInstance giantnetes.ETCDInstance, instanceStatus *v1alpha1.ETCDInstanceBackupStatusIndex) bool {
	// If state is terminal, there's nothing else we can do on this instance, so just skip to next one.
	if isTerminalInstaceState(instanceStatus.V2.Status) {
		return false
	}

	if instanceStatus.V2.StartedTimestamp.Time.IsZero() {
		instanceStatus.V2.StartedTimestamp.Time = time.Now().UTC()
	}

	etcdSettings := etcdInstance.ETCDv2
	if etcdSettings.AreComplete() {
		r.logger.LogCtx(ctx, "level", "debug", "message", fmt.Sprintf("Starting v2 backup on instance %s", instanceStatus.Name))

		backupper := etcd.NewV2Backup(etcdInstance.ETCDv2.DataDir, r.encryptionPwd, r.logger, key.FilenamePrefix(instanceStatus.Name))

		backupAttemptResult, err := r.performBackup(ctx, backupper, instanceStatus.Name)
		if err == nil {
			// Backup was successful.
			instanceStatus.V2.LatestError = ""
			instanceStatus.V2.Status = instanceBackupStateCompleted
		} else {
			// Backup was unsuccessful.
			instanceStatus.V2.LatestError = err.Error()
			instanceStatus.V2.Status = instanceBackupStateFailed
		}

		r.metricsHolder.Add(instanceStatus.Name, backupper.Version(), backupAttemptResult)
	} else {
		r.logger.LogCtx(ctx, "level", "info", "message", fmt.Sprintf("V2 backup skipped for %s because ETCD V2 setting are not set.", instanceStatus.Name))
		instanceStatus.V2.Status = instanceBackupStateSkipped
	}

	instanceStatus.V2.FinishedTimestamp = v1alpha1.DeepCopyTime{
		Time: time.Now().UTC(),
	}

	return true
}
