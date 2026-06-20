package jsonrpc

import (
	"context"

	"github.com/komari-monitor/komari/database/auditlog"
	d_ddns "github.com/komari-monitor/komari/database/ddns"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/pkg/rpc"
)

func init() {
	reg("getDDNSProvider", adminGetDDNSProvider, "Get DDNS provider status")
	reg("setDDNSProvider", adminSetDDNSProvider, "Set DDNS provider token")
	reg("removeDDNSToken", adminRemoveDDNSToken, "Remove DDNS provider token")
	reg("listDDNSRecords", adminListDDNSRecords, "List DDNS records")
	reg("saveDDNSRecord", adminSaveDDNSRecord, "Create or update a DDNS record")
	reg("deleteDDNSRecords", adminDeleteDDNSRecords, "Delete DDNS records")
	reg("enableDDNSRecords", adminEnableDDNSRecords, "Enable DDNS records")
	reg("disableDDNSRecords", adminDisableDDNSRecords, "Disable DDNS records")
	reg("syncDDNSRecords", adminSyncDDNSRecords, "Sync DDNS records now")
}

func adminGetDDNSProvider(_ context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Provider string `json:"provider"`
	}
	req.BindParams(&params)
	status, err := d_ddns.GetProviderStatus(params.Provider)
	if err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, err.Error(), nil)
	}
	return status, nil
}

func adminSetDDNSProvider(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Provider string `json:"provider"`
		Token    string `json:"token"`
	}
	req.BindParams(&params)
	if err := d_ddns.SaveProviderToken(params.Provider, params.Token); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "set ddns provider token", "warn")
	status, _ := d_ddns.GetProviderStatus(params.Provider)
	return status, nil
}

func adminRemoveDDNSToken(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		Provider string `json:"provider"`
	}
	req.BindParams(&params)
	if err := d_ddns.RemoveProviderToken(params.Provider); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "remove ddns provider token", "warn")
	return nil, nil
}

func adminListDDNSRecords(_ context.Context, _ *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	records, err := d_ddns.ListRecords()
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return records, nil
}

func adminSaveDDNSRecord(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var record models.DDNSRecord
	if err := req.BindParams(&record); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request body: "+err.Error(), nil)
	}
	if err := d_ddns.SaveRecord(&record); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "save ddns record", "info")
	return record, nil
}

func adminDeleteDDNSRecords(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	ids, rpcErr := bindDDNSIds(req)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if err := d_ddns.DeleteRecords(ids); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	auditlog.Log(ip, actor, "delete ddns records", "warn")
	return nil, nil
}

func adminEnableDDNSRecords(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	return adminSetDDNSRecordsEnabled(ctx, req, true)
}

func adminDisableDDNSRecords(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	return adminSetDDNSRecordsEnabled(ctx, req, false)
}

func adminSetDDNSRecordsEnabled(ctx context.Context, req *rpc.JsonRpcRequest, enabled bool) (any, *rpc.JsonRpcError) {
	ids, rpcErr := bindDDNSIds(req)
	if rpcErr != nil {
		return nil, rpcErr
	}
	if err := d_ddns.SetRecordsEnabled(ids, enabled); err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	actor, ip := auditActor(ctx)
	if enabled {
		auditlog.Log(ip, actor, "enable ddns records", "info")
	} else {
		auditlog.Log(ip, actor, "disable ddns records", "warn")
	}
	return nil, nil
}

func adminSyncDDNSRecords(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	var params struct {
		ID    uint `json:"id"`
		Force bool `json:"force"`
	}
	req.BindParams(&params)
	if params.ID != 0 {
		result, err := d_ddns.SyncOne(ctx, params.ID, params.Force)
		if err != nil {
			return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
		}
		return result, nil
	}
	results, err := d_ddns.SyncAll(ctx)
	if err != nil {
		return nil, rpc.MakeError(rpc.InternalError, err.Error(), nil)
	}
	return results, nil
}

func bindDDNSIds(req *rpc.JsonRpcRequest) ([]uint, *rpc.JsonRpcError) {
	var params struct {
		ID  uint   `json:"id"`
		IDs []uint `json:"ids"`
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request body: "+err.Error(), nil)
	}
	if params.ID != 0 {
		params.IDs = append(params.IDs, params.ID)
	}
	if len(params.IDs) == 0 {
		return nil, rpc.MakeError(rpc.InvalidParams, "id is required", nil)
	}
	return params.IDs, nil
}
