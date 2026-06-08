import { useEffect, useMemo, useState, useCallback } from 'react';
import { useRouter } from 'next/router';
import { Box, Stack, Typography, CircularProgress } from '@mui/material';
import { ListingLayout } from '@ui/ListingLayout';
import { Button as DsButton } from '@ui/Button';
import { Label } from '@ui/Label';
import { Modal } from '@ui/Modal';
import CustomTable from '@shared/tables/CustomTable2';
import ThreeDotsMenu from '@shared/ThreeDotsMenu';
import Text from '@shared/format/Text';
import Datetime from '@shared/format/Datetime';
import CloudProviderIcon from '@shared/CloudIcon';
import apiIntegrations from '@api1/integrations';
import { isTenantAdmin } from '@lib/auth';
import { action } from 'src/utils/actionStyles';
import { ds } from 'src/utils/colors';

const GOOGLE_CHAT_SPACE_TYPE = 'google_chat_space';
const SPACE_ID_PATTERN = /^spaces\/[A-Za-z0-9_-]+$/;
// Where a Workspace admin authorizes the bot's chat.app.memberships scope (we can
// only guide + observe; Google performs the actual grant).
const GOOGLE_ADMIN_APPS_URL = 'https://admin.google.com/ac/apps/unified';

function parseConfigValues(configValues) {
  try {
    const arr = typeof configValues === 'string' ? JSON.parse(configValues) : configValues;
    if (Array.isArray(arr)) {
      return Object.fromEntries(arr.map((c) => [c?.name, c?.value]));
    }
  } catch {
    // fall through
  }
  return {};
}

const PERMISSION_TONE = { authorized: 'success', needs_authorization: 'warning', no_spaces: 'neutral' };
const PERMISSION_TEXT = {
  authorized: 'Join permission: authorized',
  needs_authorization: 'Join permission: needs admin authorization',
  no_spaces: 'Join permission: connect a space to check',
  unknown: 'Join permission: unknown',
};

/**
 * Google Chat integration panel (service-account model), styled to match the
 * Slack / MS Teams tiles. Manages the spaces bound to this tenant
 * (google_chat_space integrations): connect (via the bot's Connect-card
 * deep-link), unbind, choose the default space for notifications, and observe /
 * guide the bot's self-join permission.
 */
export default function GoogleChatSpacesPanel() {
  const router = useRouter();
  const incomingSpaceId = typeof router.query.space_id === 'string' ? router.query.space_id : '';
  const incomingDisplayName = typeof router.query.display_name === 'string' ? router.query.display_name : '';

  const userIsTenantAdmin = isTenantAdmin();

  const [spaces, setSpaces] = useState([]);
  const [loading, setLoading] = useState(true);
  const [connecting, setConnecting] = useState(false);
  const [busyId, setBusyId] = useState('');
  const [toDelete, setToDelete] = useState(null);
  const [notice, setNotice] = useState(null);
  const [permissionStatus, setPermissionStatus] = useState('unknown');
  const [checkingPermission, setCheckingPermission] = useState(false);

  const loadSpaces = useCallback(async () => {
    try {
      const res = await apiIntegrations.listIntegrations({ type: GOOGLE_CHAT_SPACE_TYPE, limit: 200 });
      const rows = res?.data?.data?.integrations_list?.rows || [];
      setSpaces(
        rows.map((row) => {
          const cfg = parseConfigValues(row.integration_config_values);
          return {
            id: row.id,
            spaceId: row.name,
            displayName: cfg.display_name || row.name,
            boundBy: row.created_by_display_name || '—',
            boundAt: row.created_at,
            status: row.status,
            isDefault: cfg.is_default === 'true',
          };
        })
      );
    } catch (error) {
      console.error('Failed to load Google Chat spaces:', error);
      setSpaces([]);
    } finally {
      setLoading(false);
    }
  }, []);

  const checkPermission = useCallback(async () => {
    setCheckingPermission(true);
    try {
      const res = await apiIntegrations.getGoogleChatPermissionStatus();
      setPermissionStatus(res?.status || 'unknown');
    } catch {
      setPermissionStatus('unknown');
    } finally {
      setCheckingPermission(false);
    }
  }, []);

  useEffect(() => {
    loadSpaces();
    checkPermission();
  }, [loadSpaces, checkPermission]);

  const incomingValid = useMemo(() => SPACE_ID_PATTERN.test(incomingSpaceId), [incomingSpaceId]);
  const alreadyBound = useMemo(() => spaces.some((s) => s.spaceId === incomingSpaceId), [spaces, incomingSpaceId]);
  const showPendingRow = incomingValid && !alreadyBound && !loading;

  const upsertConfig = (spaceId, values) =>
    apiIntegrations.addIntegrations({
      integration_name: GOOGLE_CHAT_SPACE_TYPE,
      integration_config_name: spaceId,
      integration_config_values: values,
      account_ids: [],
      source: 'user',
      skip_validation: true,
    });

  const handleConnect = async () => {
    setConnecting(true);
    setNotice(null);
    try {
      const response = await upsertConfig(incomingSpaceId, [
        { name: 'display_name', value: incomingDisplayName },
        { name: 'space_type', value: 'SPACE' },
      ]);
      const gqlError = response?.errors?.[0]?.message || response?.data?.errors?.[0]?.message || response?.message || '';
      if (gqlError) {
        if (gqlError.toLowerCase().includes('already exists')) {
          setNotice({ tone: 'warning', text: 'This space is already bound to a Nudgebee organization.' });
        } else {
          setNotice({ tone: 'critical', text: gqlError });
        }
      } else if (response?.data?.data?.integrations_create_config?.id) {
        setNotice({ tone: 'success', text: 'Space connected. Return to Google Chat and retry your message.' });
      }
      await loadSpaces();
    } catch (err) {
      setNotice({ tone: 'critical', text: err?.message || 'Failed to connect this space.' });
    } finally {
      setConnecting(false);
    }
  };

  // Set one space as the tenant default; clear the previous default (one per tenant).
  const handleSetDefault = async (space) => {
    setBusyId(space.spaceId);
    setNotice(null);
    try {
      const previous = spaces.find((s) => s.isDefault && s.spaceId !== space.spaceId);
      await upsertConfig(space.spaceId, [{ name: 'is_default', value: 'true' }]);
      if (previous) {
        await upsertConfig(previous.spaceId, [{ name: 'is_default', value: 'false' }]);
      }
      setNotice({ tone: 'success', text: `“${space.displayName}” is now the default space for notifications.` });
      await loadSpaces();
    } catch (err) {
      setNotice({ tone: 'critical', text: err?.message || 'Failed to set the default space.' });
    } finally {
      setBusyId('');
    }
  };

  const handleRemoveDefault = async (space) => {
    setBusyId(space.spaceId);
    setNotice(null);
    try {
      await upsertConfig(space.spaceId, [{ name: 'is_default', value: 'false' }]);
      setNotice({ tone: 'success', text: `Removed “${space.displayName}” as the default space.` });
      await loadSpaces();
    } catch (err) {
      setNotice({ tone: 'critical', text: err?.message || 'Failed to remove the default space.' });
    } finally {
      setBusyId('');
    }
  };

  const handleDelete = async () => {
    if (!toDelete) return;
    setBusyId(toDelete.spaceId);
    try {
      const response = await apiIntegrations.deleteIntegrations({
        integration_name: GOOGLE_CHAT_SPACE_TYPE,
        integration_config_name: toDelete.spaceId,
        source: 'user',
      });
      const gqlError = response?.errors?.[0]?.message || response?.data?.errors?.[0]?.message || response?.message || '';
      if (gqlError) {
        setNotice({ tone: 'critical', text: gqlError });
      } else {
        setNotice({ tone: 'success', text: `Unbound “${toDelete.displayName}”.` });
      }
      await loadSpaces();
    } catch (err) {
      setNotice({ tone: 'critical', text: err?.message || 'Failed to unbind this space.' });
    } finally {
      setBusyId('');
      setToDelete(null);
    }
  };

  const getMenuItems = (space) => {
    if (!userIsTenantAdmin) return [];
    const items = space.isDefault ? [{ label: 'Remove as default', id: 'remove_default' }] : [{ label: 'Set as default', id: 'set_default' }];
    items.push({ label: 'Unbind', id: 'unbind' });
    return items;
  };

  const onMenuClick = (menuItem, space) => {
    if (menuItem.id === 'set_default') handleSetDefault(space);
    else if (menuItem.id === 'remove_default') handleRemoveDefault(space);
    else if (menuItem.id === 'unbind') setToDelete(space);
  };

  const headers = ['Space', 'Bound by', 'Bound at', 'Status', 'Default', ''];

  const tableData = useMemo(
    () =>
      spaces.map((space) => [
        {
          component: (
            <Box sx={{ minWidth: 0 }}>
              <Text value={space.displayName} />
              <Typography variant='caption' sx={{ color: ds.gray[600], display: 'block' }} noWrap title={space.spaceId}>
                {space.spaceId}
              </Typography>
            </Box>
          ),
        },
        { component: <Text value={space.boundBy} /> },
        { component: <Datetime value={space.boundAt} /> },
        {
          component: (
            <Label tone={space.status === 'disabled' ? 'neutral' : 'success'} text={space.status === 'disabled' ? 'Disabled' : 'Active'} size='sm' />
          ),
        },
        { component: space.isDefault ? <Label tone='info' text='Default' size='sm' /> : <Text value='—' /> },
        {
          component:
            userIsTenantAdmin && busyId !== space.spaceId ? (
              <ThreeDotsMenu sx={{ ...action.primary }} menuItems={getMenuItems(space)} data={space} onMenuClick={onMenuClick} />
            ) : busyId === space.spaceId ? (
              <CircularProgress size={16} />
            ) : (
              <></>
            ),
        },
      ]),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [spaces, busyId, userIsTenantAdmin]
  );

  const permissionTone = PERMISSION_TONE[permissionStatus] || 'neutral';

  return (
    <>
      <ListingLayout id='google-chat-spaces'>
        <ListingLayout.Toolbar
          actions={
            <Stack direction='row' spacing={1} alignItems='center'>
              {checkingPermission ? (
                <CircularProgress size={16} />
              ) : (
                <Label tone={permissionTone} text={PERMISSION_TEXT[permissionStatus]} size='sm' />
              )}
              {userIsTenantAdmin ? (
                <>
                  <DsButton
                    id='gchat-authorize-btn'
                    tone='secondary'
                    size='md'
                    onClick={() => window.open(GOOGLE_ADMIN_APPS_URL, '_blank', 'noopener')}
                  >
                    Authorize in Google Workspace
                  </DsButton>
                  <DsButton id='gchat-recheck-permission-btn' tone='secondary' size='md' loading={checkingPermission} onClick={checkPermission}>
                    Re-check
                  </DsButton>
                </>
              ) : null}
            </Stack>
          }
        >
          <Stack direction='row' alignItems='center' spacing={ds.space[2]}>
            <CloudProviderIcon cloud_provider='GOOGLE_CHAT' />
            <Box display='flex' flexDirection='column'>
              <Typography sx={{ fontFamily: 'var(--ds-font-display)' }} fontSize={ds.text.title} fontWeight={ds.weight.semibold} color={ds.gray[700]}>
                Google Chat
              </Typography>
              <Typography fontSize={ds.text.caption} color={ds.gray[600]}>
                Connects through the Nudgebee bot — no user sign-in. Add the bot to a space; when it posts the Connect card, a tenant admin binds the
                space here. Pick a default for notifications that don&apos;t name a space.
              </Typography>
            </Box>
          </Stack>
        </ListingLayout.Toolbar>
        <ListingLayout.Body>
          {notice ? (
            <Box sx={{ mb: 2 }}>
              <Label tone={notice.tone} text={notice.text} size='md' />
            </Box>
          ) : null}

          {showPendingRow ? (
            <Box sx={{ p: 2, mb: 2, border: '1px solid', borderColor: 'warning.main', borderRadius: 1 }} data-testid='gchat-pending-row'>
              <Stack direction='row' alignItems='center' justifyContent='space-between' spacing={2}>
                <Box>
                  <Stack direction='row' spacing={1} alignItems='center'>
                    <Typography variant='subtitle2'>{incomingDisplayName || incomingSpaceId}</Typography>
                    <Label tone='warning' text='Pending' size='sm' />
                  </Stack>
                  <Typography variant='caption' sx={{ color: ds.gray[600] }}>
                    {incomingSpaceId}
                  </Typography>
                </Box>
                {userIsTenantAdmin ? (
                  <DsButton data-testid='gchat-connect-submit' disabled={connecting} onClick={handleConnect}>
                    {connecting ? <CircularProgress size={16} /> : 'Connect'}
                  </DsButton>
                ) : (
                  <Typography variant='caption' sx={{ color: ds.gray[600] }}>
                    Ask a tenant admin to connect
                  </Typography>
                )}
              </Stack>
            </Box>
          ) : null}

          <CustomTable id='google-chat' headers={headers} tableData={tableData} loading={loading} />
        </ListingLayout.Body>
      </ListingLayout>

      <Modal
        open={!!toDelete}
        onClose={() => setToDelete(null)}
        title='Unbind Google Chat space?'
        subtitle={toDelete ? `${toDelete.displayName} (${toDelete.spaceId})` : ''}
        confirmText='Unbind'
        cancelText='Cancel'
        loading={!!busyId}
        onConfirm={handleDelete}
      >
        <Typography variant='body2'>
          Notifications routed to this space will stop, and the bot will be disconnected from it. You can reconnect later from the space&apos;s
          Connect card.
        </Typography>
      </Modal>
    </>
  );
}
