import React from 'react';
import { Box, Typography } from '@mui/material';
import { useRouter } from 'next/router';
import { withAuth, getUserSession } from '@lib/auth';
import ErrorBoundary from '@components1/common/ErrorBoundary';
import TaskRunner from '@components1/workflow/TaskRunner';

const TaskRunnerPage = () => {
  const router = useRouter();
  const accountId = (router?.query?.accountId as string) || '';

  const sessionRoles: string[] = getUserSession()?.roles || [];
  const hasAccess = sessionRoles.includes('tenant_admin') || sessionRoles.includes('account_admin');

  if (!hasAccess) {
    return (
      <Box sx={{ p: 4, textAlign: 'center' }}>
        <Typography variant='h6' color='text.secondary'>
          Access Denied
        </Typography>
        <Typography variant='body2' color='text.secondary' sx={{ mt: 1 }}>
          This page is only accessible to tenant admins and account admins.
        </Typography>
      </Box>
    );
  }

  return (
    <ErrorBoundary>
      <Box sx={{ height: 'calc(100vh - 120px)' }}>
        <TaskRunner accountId={accountId} />
      </Box>
    </ErrorBoundary>
  );
};

<<<<<<< HEAD
export default withAuth(TaskRunnerPage);
=======
export default withAuth(TaskRunnerPage);
>>>>>>> fix/task-runner-clean
