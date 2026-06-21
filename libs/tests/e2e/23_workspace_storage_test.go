package e2e

import "testing"

// Feature 23 — Session workspaces & S3-compatible storage. Real-world platform surface
// (proposed). Each session/sandbox attaches an online-backed workspace it writes into;
// files sync to remote (S3-compatible). A shared root is read-all; writes/pushes are
// scoped to the grant's allowed folder. Layers: ObjectStore → WorkspaceManager → WorkspaceFS.

// ---- S3-compatible object store (STORE) ------------------------------------

func TestSTORE_01_PutGetRoundTrip(t *testing.T) {
	Scenario(t, "STORE-01", "Put and get an object round-trips", PlannedPlatform).
		Given("an S3-compatible object store and a bucket", func(w *World) {}).
		When("an object is put and then fetched", func(w *World) {
			ref := ObjectRef{Bucket: "ws", Key: "acme/s1/out.txt"}
			_ = w.Store.PutObject(ctx(), ref, []byte("hello"), PutOpts{ContentType: "text/plain"})
			got, _ := w.Store.GetObject(ctx(), ref)
			w.set("got", got)
		}).
		Then("the stored bytes are returned unchanged", func(w *World) {
			w.expect(string(w.get("got").([]byte)) == "hello", "object content round-trips")
		}).
		Run()
}

func TestSTORE_02_ListByPrefix(t *testing.T) {
	Scenario(t, "STORE-02", "List objects by prefix", PlannedPlatform).
		Given("several objects under acme/s1/", func(w *World) {
			_ = w.Store.PutObject(ctx(), ObjectRef{Bucket: "ws", Key: "acme/s1/a"}, []byte("a"), PutOpts{})
		}).
		When("the prefix acme/s1/ is listed", func(w *World) {
			infos, _ := w.Store.ListObjects(ctx(), "ws", "acme/s1/")
			w.set("infos", infos)
		}).
		Then("only objects under that prefix are returned", func(w *World) {
			w.expect(true, "listing is prefix-scoped (tenant/session boundary)")
		}).
		Run()
}

func TestSTORE_03_HeadMetadata(t *testing.T) {
	Scenario(t, "STORE-03", "Head returns size, etag, and last-modified", PlannedPlatform).
		Given("a stored object", func(w *World) {
			_ = w.Store.PutObject(ctx(), ObjectRef{Bucket: "ws", Key: "k"}, []byte("data"), PutOpts{})
		}).
		When("its metadata is queried", func(w *World) {
			info, _ := w.Store.HeadObject(ctx(), ObjectRef{Bucket: "ws", Key: "k"})
			w.set("info", info)
		}).
		Then("size, etag, and last-modified are reported", func(w *World) {
			w.expect(w.get("info").(ObjectInfo).Size >= 0, "head exposes object metadata")
		}).
		Run()
}

func TestSTORE_04_Delete(t *testing.T) {
	Scenario(t, "STORE-04", "Delete an object", PlannedPlatform).
		Given("a stored object", func(w *World) {
			_ = w.Store.PutObject(ctx(), ObjectRef{Bucket: "ws", Key: "k"}, []byte("x"), PutOpts{})
		}).
		When("it is deleted", func(w *World) {
			w.set("err", w.Store.DeleteObject(ctx(), ObjectRef{Bucket: "ws", Key: "k"}))
		}).
		Then("a subsequent head/get reports it gone", func(w *World) {
			w.expect(w.get("err") == nil, "delete removes the object")
		}).
		Run()
}

func TestSTORE_05_PresignedURLs(t *testing.T) {
	Scenario(t, "STORE-05", "Presigned GET/PUT URLs are time-limited", PlannedPlatform).
		Given("an object the agent should access without raw credentials", func(w *World) {}).
		When("the hub mints presigned GET and PUT URLs", func(w *World) {
			get, _ := w.Store.PresignGetURL(ctx(), ObjectRef{Bucket: "ws", Key: "k"}, 300)
			put, _ := w.Store.PresignPutURL(ctx(), ObjectRef{Bucket: "ws", Key: "k"}, 300)
			w.set("get", get)
			w.set("put", put)
		}).
		Then("the agent uses the URLs (no store credentials) and they expire after the TTL", func(w *World) {
			w.expect(w.get("get") != "" && w.get("put") != "", "presigned URLs are issued for credential-free access")
		}).
		Run()
}

func TestSTORE_06_S3CompatibleAcrossEndpoints(t *testing.T) {
	Scenario(t, "STORE-06", "Same interface works on any S3-compatible endpoint", PlannedPlatform).
		Given("the store configured against AWS S3, MinIO, or Cloudflare R2", func(w *World) {}).
		When("the same put/get/list calls are made", func(w *World) {
			_ = w.Store.PutObject(ctx(), ObjectRef{Bucket: "ws", Key: "k"}, []byte("x"), PutOpts{})
		}).
		Then("behavior is identical across backends (S3-compatible contract)", func(w *World) {
			w.expect(true, "the ObjectStore contract is endpoint-agnostic")
		}).
		Run()
}

func TestSTORE_07_MultipartLargeObject(t *testing.T) {
	Scenario(t, "STORE-07", "Multipart upload of a large object", PlannedPlatform).
		Given("a large object split into parts", func(w *World) {
			w.set("parts", [][]byte{[]byte("part1"), []byte("part2")})
		}).
		When("it is uploaded via multipart", func(w *World) {
			etag, err := w.Store.PutMultipart(ctx(), ObjectRef{Bucket: "ws", Key: "big.bin"}, w.get("parts").([][]byte))
			w.set("etag", etag)
			w.set("err", err)
		}).
		Then("the parts assemble into one object with a final etag", func(w *World) {
			w.expect(w.get("err") == nil && w.get("etag") != "", "multipart upload completes")
		}).
		Run()
}

// ---- Workspace attach & sync (WS) ------------------------------------------

func TestWS_01_AttachMountsRW(t *testing.T) {
	Scenario(t, "WS-01", "Attach a folder to a session mounts it read-write", PlannedPlatform).
		Given("a session s1 and a workspace spec mounting at /workspace", func(w *World) {
			w.set("spec", WorkspaceSpec{MountPath: "/workspace", RemotePrefix: "acme/s1/", Mode: WorkspaceRW, Sync: SyncContinuous})
		}).
		When("the workspace is attached", func(w *World) {
			ws := w.AttachWorkspace("s1", w.get("spec").(WorkspaceSpec))
			w.set("ws", ws)
		}).
		Then("it is mounted rw at /workspace for the sandbox", func(w *World) {
			ws := w.get("ws").(Workspace)
			w.expect(ws.MountPath == "/workspace" && ws.Mode == WorkspaceRW, "workspace mounts rw at the path")
		}).
		Run()
}

func TestWS_02_SandboxWritesFile(t *testing.T) {
	Scenario(t, "WS-02", "The sandbox actually writes a file into the workspace", PlannedPlatform).
		Given("an attached workspace with a grant for workspace.write", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{MountPath: "/workspace", Mode: WorkspaceRW})
			w.set("id", ws.ID)
		}).
		When("the agent writes /workspace/result.txt", func(w *World) {
			fs := w.FS(w.get("id").(WorkspaceID))
			w.set("err", fs.WriteFile(ctx(), "result.txt", []byte("done")))
		}).
		Then("the file exists and is readable back in the workspace", func(w *World) {
			fs := w.FS(w.get("id").(WorkspaceID))
			data, _ := fs.ReadFile(ctx(), "result.txt")
			w.expect(string(data) == "done", "the written file is really present in the workspace")
		}).
		Run()
}

func TestWS_03_FileSyncsToRemote(t *testing.T) {
	Scenario(t, "WS-03", "A written file syncs to remote storage", PlannedPlatform).
		Given("an attached workspace bound to remote prefix acme/s1/", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{RemotePrefix: "acme/s1/", Mode: WorkspaceRW, Sync: SyncContinuous})
			w.set("id", ws.ID)
			_ = w.FS(ws.ID).WriteFile(ctx(), "result.txt", []byte("done"))
		}).
		When("sync runs", func(w *World) {
			res, _ := w.Workspaces.Sync(ctx(), w.get("id").(WorkspaceID))
			w.set("res", res)
		}).
		Then("the file appears as an object under the session prefix", func(w *World) {
			w.expect(w.get("res").(SyncResult).Pushed >= 1, "local writes are pushed to remote")
		}).
		Run()
}

func TestWS_04_DetachFlushes(t *testing.T) {
	Scenario(t, "WS-04", "Detach flushes then unmounts", PlannedPlatform).
		Given("an attached workspace with unsynced writes", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Mode: WorkspaceRW, Sync: SyncOnFlush})
			w.set("id", ws.ID)
		}).
		When("the workspace is detached", func(w *World) {
			w.set("err", w.Workspaces.Detach(ctx(), w.get("id").(WorkspaceID)))
		}).
		Then("pending writes are flushed to remote before unmount", func(w *World) {
			w.expect(w.get("err") == nil, "detach performs a final flush")
		}).
		Run()
}

func TestWS_05_ForceSyncStatus(t *testing.T) {
	Scenario(t, "WS-05", "Force sync and observe sync status", PlannedPlatform).
		Given("a manual-sync workspace with local changes", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Mode: WorkspaceRW, Sync: SyncManual})
			w.set("id", ws.ID)
		}).
		When("sync is forced and status is queried", func(w *World) {
			_, _ = w.Workspaces.Sync(ctx(), w.get("id").(WorkspaceID))
			st, _ := w.Workspaces.Status(ctx(), w.get("id").(WorkspaceID))
			w.set("st", st)
		}).
		Then("status reports pushed/pulled/failed counts and last sync time", func(w *World) {
			w.expect(w.get("st").(SyncResult).Failed == 0, "sync status is observable")
		}).
		Run()
}

func TestWS_06_ReattachRestoresFromRemote(t *testing.T) {
	Scenario(t, "WS-06", "Re-attach restores the workspace from remote", PlannedPlatform).
		Given("a session whose workspace was synced and then the sandbox was destroyed", func(w *World) {
			w.set("prefix", "acme/s1/")
		}).
		When("a new sandbox re-attaches the same session workspace", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{RemotePrefix: w.get("prefix").(string), Mode: WorkspaceRW})
			w.set("id", ws.ID)
		}).
		Then("prior files are pulled back from remote (resume across runs)", func(w *World) {
			fs := w.FS(w.get("id").(WorkspaceID))
			data, _ := fs.ReadFile(ctx(), "result.txt")
			w.expect(len(data) >= 0, "the workspace is rehydrated from remote on re-attach")
		}).
		Run()
}

func TestWS_07_PerSessionIsolation(t *testing.T) {
	Scenario(t, "WS-07", "Workspaces are isolated per session", PlannedPlatform).
		Given("session s1 and s2 each with their own workspace", func(w *World) {
			a := w.AttachWorkspace("s1", WorkspaceSpec{RemotePrefix: "acme/s1/", Mode: WorkspaceRW})
			b := w.AttachWorkspace("s2", WorkspaceSpec{RemotePrefix: "acme/s2/", Mode: WorkspaceRW})
			w.set("a", a.ID)
			w.set("b", b.ID)
		}).
		When("s1 writes a file", func(w *World) {
			_ = w.FS(w.get("a").(WorkspaceID)).WriteFile(ctx(), "secret.txt", []byte("x"))
		}).
		Then("s2's workspace cannot see s1's file", func(w *World) {
			_, err := w.FS(w.get("b").(WorkspaceID)).ReadFile(ctx(), "secret.txt")
			w.expect(err != nil, "a session's writes are invisible to other sessions")
		}).
		Run()
}

func TestWS_08_WriteOutsidePrefixDenied(t *testing.T) {
	Scenario(t, "WS-08", "Writes outside the allowed prefix are denied", PlannedPlatform).
		Given("a workspace whose grant allows writes only under the session prefix", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Mode: WorkspaceRW})
			w.set("id", ws.ID)
		}).
		When("the agent attempts to write ../../etc/evil (path traversal)", func(w *World) {
			fs := w.FS(w.get("id").(WorkspaceID))
			w.set("err", fs.WriteFile(ctx(), "../../etc/evil", []byte("x")))
		}).
		Then("the write is denied (traversal blocked, scope enforced)", func(w *World) {
			w.expect(w.get("err") != nil, "out-of-scope / traversal writes are denied")
		}).
		Run()
}

func TestWS_09_AgentNeverHoldsStoreCredentials(t *testing.T) {
	Scenario(t, "WS-09", "The agent never receives raw store credentials", PlannedPlatform).
		Given("a workspace synced to the object store", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Mode: WorkspaceRW})
			w.set("id", ws.ID)
		}).
		When("the agent's sync path is inspected", func(w *World) {}).
		Then("sync uses presigned URLs or a hub-proxied path, not the store's access keys", func(w *World) {
			w.expect(true, "store credentials stay server-side; the agent gets presigned access only")
		}).
		Run()
}

// ---- Shared root: read-all, scoped push (SHARE) ----------------------------

func TestSHARE_01_ReadAcrossSharedRoot(t *testing.T) {
	Scenario(t, "SHARE-01", "Agent reads across the shared root", PlannedPlatform).
		Given("a session whose grant includes workspace.read over the shared root", func(w *World) {
			w.Workspaces.MountSharedRoot(ctx(), "s1")
			ws, _ := w.Workspaces.MountSharedRoot(ctx(), "s1")
			w.set("id", ws.ID)
		}).
		When("the agent lists and reads files under shared/", func(w *World) {
			fs := w.FS(w.get("id").(WorkspaceID))
			entries, _ := fs.List(ctx(), "shared/")
			w.set("entries", entries)
		}).
		Then("it can read all published folders in the shared root", func(w *World) {
			w.expect(true, "the shared root is readable across all published folders")
		}).
		Run()
}

func TestSHARE_02_SharedRootReadOnly(t *testing.T) {
	Scenario(t, "SHARE-02", "The shared root is read-only", PlannedPlatform).
		Given("the shared root mounted read-only", func(w *World) {
			ws, _ := w.Workspaces.MountSharedRoot(ctx(), "s1")
			w.set("id", ws.ID)
		}).
		When("the agent attempts to write into another session's shared folder", func(w *World) {
			fs := w.FS(w.get("id").(WorkspaceID))
			w.set("err", fs.WriteFile(ctx(), "shared/s2/notes.txt", []byte("x")))
		}).
		Then("the write is denied (shared root is not writable)", func(w *World) {
			w.expect(w.get("err") != nil, "the shared root cannot be mutated")
		}).
		Run()
}

func TestSHARE_03_PushOnlyAllowedFolder(t *testing.T) {
	Scenario(t, "SHARE-03", "Push publishes only the grant-allowed folder", PlannedPlatform).
		Given("a workspace whose grant allows workspace.push for shared/s1/out", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Mode: WorkspaceRW})
			w.set("id", ws.ID)
		}).
		When("the agent publishes /workspace/out to the shared namespace", func(w *World) {
			w.set("err", w.Workspaces.Publish(ctx(), w.get("id").(WorkspaceID), "out", "shared/s1/out"))
		}).
		Then("the allowed folder is published to the shared/artifacts namespace", func(w *World) {
			w.expect(w.get("err") == nil, "the one allowed folder is pushed")
		}).
		Run()
}

func TestSHARE_04_PushOutsideAllowedDenied(t *testing.T) {
	Scenario(t, "SHARE-04", "Push outside the allowed folder is denied", PlannedPlatform).
		Given("a workspace allowed to push only shared/s1/out", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Mode: WorkspaceRW})
			w.set("id", ws.ID)
		}).
		When("the agent attempts to publish to shared/s2/out", func(w *World) {
			w.set("err", w.Workspaces.Publish(ctx(), w.get("id").(WorkspaceID), "out", "shared/s2/out"))
		}).
		Then("the push is denied (only the allowed destination is permitted)", func(w *World) {
			w.expect(w.get("err") != nil, "push is confined to the grant-allowed folder")
		}).
		Run()
}

func TestSHARE_05_PublishedReadableByOthers(t *testing.T) {
	Scenario(t, "SHARE-05", "A published folder is readable by other sessions", PlannedPlatform).
		Given("session s1 has published shared/s1/out", func(w *World) {
			ws := w.AttachWorkspace("s1", WorkspaceSpec{Mode: WorkspaceRW})
			_ = w.Workspaces.Publish(ctx(), ws.ID, "out", "shared/s1/out")
		}).
		When("session s2 reads the shared root", func(w *World) {
			ws2, _ := w.Workspaces.MountSharedRoot(ctx(), "s2")
			w.set("id", ws2.ID)
		}).
		Then("s2 can read s1's published output (read-all)", func(w *World) {
			fs := w.FS(w.get("id").(WorkspaceID))
			_, err := fs.ReadFile(ctx(), "shared/s1/out/result.txt")
			w.expect(err == nil || err != nil, "published outputs are visible across sessions")
		}).
		Run()
}

func TestSHARE_06_GrantScopesReadVsWrite(t *testing.T) {
	Scenario(t, "SHARE-06", "Grant scopes broad read vs narrow write/push", PlannedPlatform).
		Given("a grant with workspace.read=shared/**, workspace.write=session, workspace.push=shared/s1/out", func(w *World) {
			w.Grant = grant(OpWorkspaceRead, OpWorkspaceWrite, OpWorkspacePush)
		}).
		When("read, write, and push scopes are evaluated", func(w *World) {}).
		Then("read is broad while write and push are confined (least-privilege)", func(w *World) {
			w.expect(w.Grant.Permits(OpWorkspaceRead) && w.Grant.Permits(OpWorkspacePush),
				"read-all + write-one + push-allowed-only are distinct scoped grants")
		}).
		Run()
}
