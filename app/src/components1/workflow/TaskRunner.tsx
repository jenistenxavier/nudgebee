import React, { useEffect, useState } from 'react';
import { Box, Typography, CircularProgress, Accordion, AccordionSummary, AccordionDetails } from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import SearchIcon from '@mui/icons-material/Search';
import { Button as DsButton } from '@components1/ds/Button';
import { Input } from '@components1/ds/Input';
import { Form } from '@components1/ds/Form';
import { Chip } from '@components1/ds/Chip';
import { Banner } from '@components1/ds/Banner';
import apiWorkflow from '@api1/workflow';

interface TaskDefinition {
  name: string;
  description: string;
  input_schema: Record<string, any>;
  output_schema: Record<string, any>;
  aliases: string[];
}

interface TaskRunnerProps {
  accountId: string;
}

const TaskRunner: React.FC<TaskRunnerProps> = ({ accountId }) => {
  const [taskDefinitions, setTaskDefinitions] = useState<TaskDefinition[]>([]);
  const [loadingTasks, setLoadingTasks] = useState(true);
  const [selectedTask, setSelectedTask] = useState<TaskDefinition | null>(null);
  const [params, setParams] = useState<Record<string, string>>({});
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<any>(null);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  useEffect(() => {
    const fetchTasks = async () => {
      setLoadingTasks(true);
      try {
        const resp = await apiWorkflow.listTaskDefinitions();
        if (resp instanceof Error) {
          setError(resp.message || 'Failed to load task definitions.');
        } else if (resp?.errors?.length) {
          setError(resp.errors[0]?.message || 'Failed to load task definitions.');
        } else {
          const tasks: TaskDefinition[] = resp?.data?.workflow_list_taskdefinitions?.tasks || [];
          setTaskDefinitions(tasks);
        }
      } catch (e) {
        setError('Failed to load task definitions.');
      } finally {
        setLoadingTasks(false);
      }
    };
    fetchTasks();
  }, []);

  const handleSelectTask = (task: TaskDefinition) => {
    setSelectedTask(task);
    setParams({});
    setResult(null);
    setError(null);
  };

  const handleParamChange = (key: string, value: string) => {
    setParams((prev) => ({ ...prev, [key]: value }));
  };

  const handleRunTask = async () => {
    if (!accountId) { setError('Account ID is missing.'); return; }
    if (!selectedTask) return;
    setRunning(true);
    setResult(null);
    setError(null);
    try {
      // Coerce param types based on input_schema
      const coercedParams: Record<string, any> = {};
      Object.entries(params).forEach(([key, value]) => {
        const fieldSchema = selectedTask.input_schema?.properties?.[key];
        if (fieldSchema?.type === 'integer') coercedParams[key] = parseInt(value, 10);
        else if (fieldSchema?.type === 'boolean') coercedParams[key] = value === 'true';
        else if (fieldSchema?.type === 'number') coercedParams[key] = parseFloat(value);
        else coercedParams[key] = value;
      });

      const resp = await apiWorkflow.triggerTask(accountId, selectedTask.name, coercedParams);
      if (resp instanceof Error) {
        setError(resp.message || 'Task execution failed.');
      } else if (resp?.errors?.length) {
        setError(resp.errors[0]?.message || 'Task execution failed.');
      } else {
        setResult(resp?.data?.workflow_execute_task ?? resp?.data);
      }
    } catch (e: any) {
      setError(e?.message || 'Task execution failed.');
    } finally {
      setRunning(false);
    }
  };

  // Fix: handle camelCase names + no-underscore names safely
  const getCategory = (name: string) => {
    if (name.includes('')) return name.split('')[0].toUpperCase();
    const fromCamel = name.replace(/([A-Z])/g, ' $1').split(' ')[0];
    return (fromCamel || 'OTHER').toUpperCase();
  };

  const grouped = taskDefinitions
    .filter(
      (t) =>
        t.name.toLowerCase().includes(search.toLowerCase()) ||
        t.description?.toLowerCase().includes(search.toLowerCase())
    )
    .reduce<Record<string, TaskDefinition[]>>((acc, task) => {
      const category = getCategory(task.name);
      if (!acc[category]) acc[category] = [];
      acc[category].push(task);
      return acc;
    }, {});

  const inputFields = Object.entries(selectedTask?.input_schema?.properties || {});

  return (
    <Box sx={{ display: 'flex', gap: 3, height: '100%', p: 2 }}>

      {/* LEFT PANEL — Task List */}
      <Box sx={{ width: 320, flexShrink: 0, borderRight: '1px solid var(--ds-gray-200)', pr: 2 }}>
        <Typography
          variant='h6'
          sx={{ mb: 1, fontWeight: 'var(--ds-font-weight-semibold)', fontFamily: 'var(--ds-font-display)' }}
        >
          Task Types
        </Typography>

        {/* ✅ Search: use Input with leadingIcon — SearchInput is deleted */}
        <Box sx={{ mb: 2 }}>
          <Input
            size='sm'
            leadingIcon={<SearchIcon fontSize='small' />}
            placeholder='Search tasks...'
            value={search}
            onChange={(e: any) => setSearch(e?.target?.value ?? e)}
          />
        </Box>

        {loadingTasks ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
            <CircularProgress size={24} />
          </Box>
        ) : (
          Object.entries(grouped).map(([category, tasks]) => (
            <Accordion
              key={category}
              disableGutters
              defaultExpanded={false}
              sx={{ boxShadow: 'none', border: '1px solid var(--ds-gray-200)', mb: 1 }}
            >
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography
                  variant='body2'
                  sx={{ fontWeight: 'var(--ds-font-weight-semibold)', fontFamily: 'var(--ds-font-display)' }}
                >
                  {category}
                </Typography>
                <Chip label={String(tasks.length)} variant='count' size='sm' sx={{ ml: 1 }} />
              </AccordionSummary>
              <AccordionDetails sx={{ p: 0 }}>
                {tasks.map((task) => (
                  <Box
                    key={task.name}
                    onClick={() => handleSelectTask(task)}
                    sx={{
                      px: 2, py: 1, cursor: 'pointer',
                      bgcolor: selectedTask?.name === task.name ? 'var(--ds-blue-100)' : 'transparent',
                      borderLeft: selectedTask?.name === task.name
                        ? '3px solid var(--ds-brand-600)'
                        : '3px solid transparent',
                      '&:hover': { bgcolor: 'var(--ds-gray-100)' },
                    }}
                  >
                    <Typography variant='body2' sx={{ fontWeight: 'var(--ds-font-weight-medium)' }}>
                      {task.name}
                    </Typography>
                    {task.description && (
                      <Typography variant='caption' color='text.secondary' sx={{ display: 'block' }}>
                        {task.description}
                      </Typography>
                    )}
                  </Box>
                ))}
              </AccordionDetails>
            </Accordion>
          ))
        )}
      </Box>

      {/* RIGHT PANEL — Form + Results */}
      <Box sx={{ flex: 1, overflow: 'auto' }}>
        {!selectedTask ? (
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'text.secondary' }}>
            <Typography>Select a task type from the left to get started.</Typography>
          </Box>
        ) : (
          <>
            <Typography
              variant='h6'
              sx={{ mb: 0.5, fontWeight: 'var(--ds-font-weight-semibold)', fontFamily: 'var(--ds-font-display)' }}
            >
              {selectedTask.name}
            </Typography>
            {selectedTask.description && (
              <Typography variant='body2' color='text.secondary' sx={{ mb: 2 }}>
                {selectedTask.description}
              </Typography>
            )}

            {/* ✅ Parameter Form: Form.Field owns label, Input has NO label prop */}
            <Form variant='stacked' density='default'>
              {inputFields.length === 0 ? (
                <Typography variant='body2' color='text.secondary'>
                  This task requires no input parameters.
                </Typography>
              ) : (
                inputFields.map(([key, schema]: [string, any]) => (
                  // ✅ Form.Field owns the label — Input does NOT get a label prop
                  <Form.Field
                    key={key}
                    label={key}
                    required={selectedTask.input_schema?.required?.includes(key)}
                    helperText={schema?.description || ''}
                  >
                    <Input
                      size='sm'
                      placeholder={schema?.default !== undefined ? String(schema.default) : ''}
                      value={params[key] || ''}
                      onChange={(e) => handleParamChange(key, e.target.value)}
                      // ✅ NO label prop here — Form.Field owns it
                    />
                  </Form.Field>
                ))
              )}
            </Form>

            <Box sx={{ mt: 3 }}>
              <DsButton tone='primary' size='md' onClick={handleRunTask} disabled={running}>
                {running ? <CircularProgress size={16} sx={{ mr: 1 }} /> : null}
                {running ? 'Running...' : 'Run Task'}
              </DsButton>
            </Box>

            {/* ✅ Error: Banner = persistent inline message (matches this use case) */}
            {error && (
              <Box sx={{ mt: 3 }}>
                <Banner tone='danger' title='Error' description={error} />
              </Box>
            )}

            {/* Results Panel */}
            {result !== null && (
              <Box sx={{ mt: 3 }}>
                <Typography
                  variant='subtitle2'
                  sx={{ mb: 1, fontWeight: 'var(--ds-font-weight-semibold)', fontFamily: 'var(--ds-font-display)' }}
                >
                  Execution Output
                </Typography>
                <Box
                  component='pre'
                  sx={{
                    bgcolor: 'var(--ds-background-200)',
                    borderRadius: 'var(--ds-radius-md)',
                    p: 2,
                    overflow: 'auto',
                    fontSize: 'var(--ds-text-body)',
                    maxHeight: 400,
                    border: '1px solid var(--ds-gray-200)',
                    fontFamily: 'var(--ds-font-mono)',
                  }}
                >
                  {JSON.stringify(result, null, 2)}
                </Box>
              </Box>
            )}
          </>
        )}
      </Box>
    </Box>
  );
};

export default TaskRunner;