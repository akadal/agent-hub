import { BrowserRouter, Navigate, Route, Routes } from 'react-router'

import { AppShell } from '@/components/layout/app-shell'
import { AuthProvider } from '@/lib/auth'
import { AuditPage } from '@/pages/audit'
import { DashboardPage } from '@/pages/dashboard'
import { LoginPage } from '@/pages/login'
import { MachinesPage } from '@/pages/machines'
import { PermissionsPage } from '@/pages/permissions'
import { SettingsPage } from '@/pages/settings'
import { UsersPage } from '@/pages/users'
import { WorkspacePage } from '@/pages/workspace'

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route element={<AppShell />}>
            <Route index element={<DashboardPage />} />
            <Route path="workspace" element={<WorkspacePage />} />
            <Route path="workspace/:machineId" element={<WorkspacePage />} />
            <Route path="machines" element={<MachinesPage />} />
            <Route path="terminals" element={<Navigate to="/workspace" replace />} />
            <Route
              path="terminals/:id"
              element={<Navigate to="/workspace" replace />}
            />
            <Route path="users" element={<UsersPage />} />
            <Route path="permissions" element={<PermissionsPage />} />
            <Route path="audit" element={<AuditPage />} />
            <Route path="settings" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
