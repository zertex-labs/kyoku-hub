/**
 * Ref-counted body scroll lock. While at least one consumer is active the
 * page behind cannot scroll; the lost scrollbar width is compensated with
 * padding so content doesn't shift.
 */
let lockCount = 0;

function applyLock() {
  if (++lockCount > 1) return;
  const scrollbarWidth = window.innerWidth - document.documentElement.clientWidth;
  document.body.style.overflow = "hidden";
  if (scrollbarWidth > 0) {
    document.body.style.paddingRight = `${scrollbarWidth}px`;
  }
}

function releaseLock() {
  if (lockCount === 0 || --lockCount > 0) return;
  document.body.style.overflow = "";
  document.body.style.paddingRight = "";
}

export function useBodyScrollLock(active: Ref<boolean>) {
  if (import.meta.server) return;

  let locked = false;

  watch(
    active,
    (val) => {
      if (val && !locked) {
        locked = true;
        applyLock();
      } else if (!val && locked) {
        locked = false;
        releaseLock();
      }
    },
    { immediate: true },
  );

  // Unlock if the owning component unmounts while still open (e.g. page nav).
  onScopeDispose(() => {
    if (locked) {
      locked = false;
      releaseLock();
    }
  });
}
