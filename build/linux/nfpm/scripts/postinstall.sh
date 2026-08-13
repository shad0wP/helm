#!/bin/sh

# Update desktop database for .desktop file changes
# This makes the application appear in application menus and registers its capabilities.
if command -v update-desktop-database >/dev/null 2>&1; then
  echo "Updating desktop database..."
  update-desktop-database -q /usr/share/applications
else
  echo "Warning: update-desktop-database command not found. Desktop file may not be immediately recognized." >&2
fi

# Update MIME database for custom URL schemes (x-scheme-handler)
# This ensures the system knows how to handle your custom protocols.
if command -v update-mime-database >/dev/null 2>&1; then
  echo "Updating MIME database..."
  update-mime-database -n /usr/share/mime
else
  echo "Warning: update-mime-database command not found. Custom URL schemes may not be immediately recognized." >&2
fi

# Install the polkit rule so wheel/sudo-group users can toggle Helm's systemd
# system services without a password. Uses the default service set here (no
# user config exists at package-install time); after editing
# ~/.config/helm/services.json, refresh it with: sudo helm-cli setup-polkit
if [ -x /usr/local/bin/helm-cli ]; then
  /usr/local/bin/helm-cli setup-polkit || \
    echo "Note: run 'sudo helm-cli setup-polkit' to enable passwordless service control." >&2
fi

exit 0
