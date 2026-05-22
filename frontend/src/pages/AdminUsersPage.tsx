import { useState, useEffect, useRef, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { User as UserIcon, Plus, Trash2, Loader2, Shield, Eye, EyeOff, HelpCircle, X } from "lucide-react";
import { api, type User } from "../lib/api";
import { useUser } from "../contexts/UserContext";
import { useToast } from "../contexts/ToastContext";
import { useHelp } from "../contexts/HelpContext";

export default function AdminUsersPage() {
  const { user: currentUser, isAdmin } = useUser();
  const navigate = useNavigate();
  const toast = useToast();
  const { showHelp } = useHelp();

  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  // Create form state
  const [showForm, setShowForm] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newDisplayName, setNewDisplayName] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");

  // Delete confirmation modal
  const [deleteConfirm, setDeleteConfirm] = useState<{ userId: number; username: string } | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);

  // Redirect non-admins — use useEffect for consistent behavior
  useEffect(() => {
    if (!isAdmin) {
      navigate("/settings", { replace: true });
    }
  }, [isAdmin, navigate]);

  // Load users — only if admin (B1 fix: guard with admin check)
  useEffect(() => {
    if (!isAdmin) return;
    let cancelled = false;
    api.users()
      .then((data) => {
        if (!cancelled) { setUsers(data); setError(""); }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err?.message || "Failed to load users");
          toast.error(err?.message || "Failed to load users");
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [isAdmin]);

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    const username = newUsername.trim();
    const displayName = newDisplayName.trim() || username;
    if (!username || !newPassword) {
      setCreateError("Username and password are required.");
      return;
    }
    if (username.length < 2) {
      setCreateError("Username must be at least 2 characters.");
      return;
    }
    if (newPassword.length < 4) {
      setCreateError("Password must be at least 4 characters.");
      return;
    }
    setCreateError("");
    setCreating(true);
    try {
      const data = await api.createUser(username, newPassword, displayName);
      // B2 fix: full refetch after creation for data consistency
      const refreshed = await api.users();
      setUsers(refreshed);
      setNewUsername("");
      setNewDisplayName("");
      setNewPassword("");
      setShowForm(false);
      toast.success(`Account created for ${data.user.display_name || data.user.username}`);
    } catch (err: any) {
      setCreateError(err?.message || "Failed to create user");
    } finally {
      setCreating(false);
    }
  }

  async function handleDelete(userId: number, username: string) {
    if (userId === currentUser?.id) {
      toast.error("You cannot delete your own account.");
      setDeletingId(null);
      setDeleteConfirm(null);
      return;
    }
    setDeletingId(userId);
    try {
      await api.deleteUser(userId);
      setUsers((prev) => prev.filter((u) => u.id !== userId));
      toast.success(`Deleted account: ${username}`);
    } catch (err: any) {
      toast.error(err?.message || "Failed to delete user");
    } finally {
      setDeletingId(null);
      setDeleteConfirm(null);
    }
  }

  function handleCancelForm() {
    setShowForm(false);
    setNewUsername("");
    setNewDisplayName("");
    setNewPassword("");
    setCreateError("");
  }

  if (!isAdmin) return null;

  return (
    <div className="max-w-2xl">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-xl font-semibold text-text">Family Accounts</h1>
          <p className="text-sm text-muted mt-1">
            Manage who has access to Lexicon on this computer.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {/* M9: Help button */}
          <button
            onClick={() => showHelp("users.management")}
            className="p-2 text-muted hover:text-accent transition-colors rounded-md hover:bg-panel2/50"
            aria-label="Help: User management"
            title="Learn about user management"
          >
            <HelpCircle size={18} />
          </button>
          <button
            onClick={() => { setShowForm((v) => !v); setCreateError(""); }}
            className="flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-accent text-bg hover:bg-accent/90 transition-colors"
          >
            <Plus size={16} />
            Add Account
          </button>
        </div>
      </div>

      {/* Create form */}
      {showForm && (
        <form
          onSubmit={handleCreate}
          className="bg-panel border border-panel2 rounded-xl p-5 mb-6 space-y-4"
          aria-label="Create new family account"
        >
          <h2 className="text-sm font-medium text-text">New Family Account</h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label htmlFor="new-username" className="block text-xs text-muted mb-1">Username *</label>
              <input
                id="new-username"
                type="text"
                value={newUsername}
                onChange={(e) => { setNewUsername(e.target.value); setCreateError(""); }}
                className="w-full bg-bg border border-panel2 rounded-md px-3 py-2 text-sm text-text placeholder:text-muted/50 focus:outline-none focus:border-accent/40"
                placeholder="e.g. alice"
                autoFocus
                aria-required="true"
              />
            </div>
            <div>
              <label htmlFor="new-displayname" className="block text-xs text-muted mb-1">Display Name</label>
              <input
                id="new-displayname"
                type="text"
                value={newDisplayName}
                onChange={(e) => setNewDisplayName(e.target.value)}
                className="w-full bg-bg border border-panel2 rounded-md px-3 py-2 text-sm text-text placeholder:text-muted/50 focus:outline-none focus:border-accent/40"
                placeholder="e.g. Alice"
              />
            </div>
          </div>
          <div>
            <label htmlFor="new-password" className="block text-xs text-muted mb-1">Password *</label>
            <div className="relative">
              <input
                id="new-password"
                type={showNewPassword ? "text" : "password"}
                value={newPassword}
                onChange={(e) => { setNewPassword(e.target.value); setCreateError(""); }}
                className="w-full bg-bg border border-panel2 rounded-md px-3 py-2 pr-9 text-sm text-text placeholder:text-muted/50 focus:outline-none focus:border-accent/40"
                placeholder="At least 4 characters"
                aria-required="true"
              />
              <button
                type="button"
                onClick={() => setShowNewPassword((p) => !p)}
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted hover:text-text"
                tabIndex={-1}
                aria-label={showNewPassword ? "Hide password" : "Show password"}
                aria-pressed={showNewPassword}
              >
                {showNewPassword ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            </div>
          </div>
          {createError && (
            <p className="text-xs text-red-400 bg-red-400/10 border border-red-400/20 rounded-md px-3 py-2" role="alert">
              {createError}
            </p>
          )}
          <div className="flex gap-3">
            <button
              type="submit"
              disabled={creating}
              className="flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-accent text-bg hover:bg-accent/90 disabled:opacity-50 transition-colors"
            >
              {creating ? <Loader2 size={16} className="animate-spin" /> : null}
              Create Account
            </button>
            <button
              type="button"
              onClick={handleCancelForm}
              className="px-4 py-2 rounded-md text-sm text-muted hover:text-text border border-panel2 hover:border-panel2/80 transition-colors"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {/* Error loading */}
      {error && !loading && (
        <p className="text-sm text-red-400 bg-red-400/10 border border-red-400/20 rounded-lg px-4 py-3 mb-4" role="alert">
          {error}
        </p>
      )}

      {/* Loading */}
      {loading && (
        <div className="flex items-center justify-center py-12 text-muted">
          <Loader2 size={20} className="animate-spin mr-2" />
          Loading accounts…
        </div>
      )}

      {/* User list */}
      {!loading && !error && (
        <div className="space-y-2" aria-live="polite" aria-atomic="false">
          {users.length === 0 ? (
            <p className="text-sm text-muted py-8 text-center">
              No family accounts yet. Create one to get started.
            </p>
          ) : (
            users.map((u) => {
              const isSelf = u.id === currentUser?.id;
              const isDeleting = deletingId === u.id;
              return (
                <div
                  key={u.id}
                  className="flex items-center gap-4 bg-panel border border-panel2 rounded-lg px-4 py-3"
                >
                  <div className="flex items-center justify-center w-9 h-9 rounded-full bg-panel2 text-muted flex-shrink-0">
                    <UserIcon size={18} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm text-text font-medium truncate">
                        {u.display_name || u.username}
                      </span>
                      {u.is_admin && (
                        <span className="flex items-center gap-0.5 text-[10px] text-accent/80 font-medium bg-accent/10 px-1.5 py-0.5 rounded">
                          <Shield size={10} />
                          ADMIN
                        </span>
                      )}
                      {isSelf && (
                        <span className="text-[10px] text-muted bg-panel2 px-1.5 py-0.5 rounded" aria-label="This is your account">
                          you
                        </span>
                      )}
                    </div>
                    <p className="text-xs text-muted truncate">@{u.username}</p>
                  </div>
                  <button
                    onClick={() => {
                      if (!isSelf) {
                        setDeleteConfirm({ userId: u.id, username: u.username });
                      }
                    }}
                    disabled={isSelf || isDeleting}
                    className="p-2 text-muted hover:text-red-400 disabled:opacity-30 disabled:cursor-not-allowed transition-colors rounded-md hover:bg-red-400/10"
                    aria-label={isSelf ? "Cannot delete your own account" : `Delete ${u.username}`}
                    title={isSelf ? "Cannot delete yourself" : `Delete ${u.username}`}
                  >
                    {isDeleting ? (
                      <Loader2 size={16} className="animate-spin" />
                    ) : (
                      <Trash2 size={16} />
                    )}
                  </button>
                </div>
              );
            })
          )}
        </div>
      )}

      {/* M7/P5: Delete confirmation modal (replaces window.confirm) */}
      {deleteConfirm && (
        <>
          <div className="fixed inset-0 z-50 bg-black/60" onClick={() => setDeleteConfirm(null)} />
          <div className="fixed inset-0 z-50 flex items-center justify-center p-4" role="alertdialog" aria-modal="true" aria-label="Confirm account deletion">
            <div className="bg-panel border border-panel2 rounded-xl p-6 max-w-sm w-full shadow-xl">
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-sm font-medium text-text">Delete Account</h3>
                <button
                  onClick={() => setDeleteConfirm(null)}
                  className="p-1 text-muted hover:text-text"
                  aria-label="Close"
                >
                  <X size={16} />
                </button>
              </div>
              <p className="text-sm text-muted mb-6">
                Are you sure you want to delete the account <strong className="text-text">"{deleteConfirm.username}"</strong>? This cannot be undone.
              </p>
              <div className="flex gap-3 justify-end">
                <button
                  onClick={() => setDeleteConfirm(null)}
                  className="px-4 py-2 rounded-md text-sm text-muted hover:text-text border border-panel2 hover:border-panel2/80 transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={() => handleDelete(deleteConfirm.userId, deleteConfirm.username)}
                  disabled={deletingId === deleteConfirm.userId}
                  className="flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-red-500 text-white hover:bg-red-600 disabled:opacity-50 transition-colors"
                >
                  {deletingId === deleteConfirm.userId ? (
                    <Loader2 size={14} className="animate-spin" />
                  ) : null}
                  Delete Account
                </button>
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
