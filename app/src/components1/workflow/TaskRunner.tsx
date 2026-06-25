import React, { useEffect, useState } from 'react';
import { Box, Typography, TextField, CircularProgress, Accordion, AccordionSummary, AccordionDetails, Chip, Alert } from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { Button as DsButton } from '@components1/ds/Button';
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
    if (!accountId) {
      setError('Account ID is missing.');
      return;
    }
    if (!selectedTask) return;
    setRunning(true);
    setResult(null);
    setError(null);
    try {
      const resp = await apiWorkflow.triggerTask(accountId, selectedTask.name, params);
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

  // Group tasks by category (prefix before first underscore, e.g. "http", "k8s")
  const grouped = taskDefinitions
    .filter((t) => t.name.toLowerCase().includes(search.toLowerCase()) || t.description?.toLowerCase().includes(search.toLowerCase()))
    .reduce<Record<string, TaskDefinition[]>>((acc, task) => {
      const category = task.name.split('_')[0].toUpperCase();
      if (!acc[category]) acc[category] = [];
      acc[category].push(task);
      return acc;
    }, {});

  const inputFields = Object.entries(selectedTask?.input_schema?.properties || {});

  return (
    <Box sx={{ display: 'flex', gap: 3, height: '100%', p: 2 }}>
      {/* LEFT PANEL — Task List */}
      <Box sx={{ width: 320, flexShrink: 0, borderRight: '1px solid #e0e0e0', pr: 2 }}>
        <Typography variant='h6' sx={{ mb: 1, fontWeight: 600 }}>
          Task Types
        </Typography>
        <TextField size='small' fullWidth placeholder='Search tasks...' value={search} onChange={(e) => setSearch(e.target.value)} sx={{ mb: 2 }} />
        {loadingTasks ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
            <CircularProgress size={24} />
          </Box>
        ) : (
          Object.entries(grouped).map(([category, tasks]) => (
            <Accordion key={category} disableGutters defaultExpanded={false} sx={{ boxShadow: 'none', border: '1px solid #e0e0e0', mb: 1 }}>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography variant='body2' fontWeight={600}>
                  {category}
                  <Chip label={tasks.length} size='small' sx={{ ml: 1 }} />
                </Typography>
              </AccordionSummary>
              <AccordionDetails sx={{ p: 0 }}>
                {tasks.map((task) => (
                  <Box
                    key={task.name}
                    onClick={() => handleSelectTask(task)}
                    sx={{
                      px: 2,
                      py: 1,
                      cursor: 'pointer',
                      bgcolor: selectedTask?.name === task.name ? '#f0f4ff' : 'transparent',
                      borderLeft: selectedTask?.name === task.name ? '3px solid #3b5bf5' : '3px solid transparent',
                      '&:hover': { bgcolor: '#f5f5f5' },
                    }}
                  >
                    <Typography variant='body2' fontWeight={500}>
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
            <Typography variant='h6' fontWeight={600} sx={{ mb: 0.5 }}>
              {selectedTask.name}
            </Typography>
            {selectedTask.description && (
              <Typography variant='body2' color='text.secondary' sx={{ mb: 2 }}>
                {selectedTask.description}
              </Typography>
            )}

            {/* Parameter Form */}
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mb: 3 }}>
              {inputFields.length === 0 ? (
                <Typography variant='body2' color='text.secondary'>
                  This task requires no input parameters.
                </Typography>
              ) : (
                inputFields.map(([key, schema]: [string, any]) => (
                  <TextField
                    key={key}
                    label={key}
                    size='small'
                    fullWidth
                    required={selectedTask.input_schema?.required?.includes(key)}
                    helperText={schema?.description || ''}
                    placeholder={schema?.default !== undefined ? String(schema.default) : ''}
                    value={params[key] || ''}
                    onChange={(e) => handleParamChange(key, e.target.value)}
                  />
                ))
              )}
            </Box>

            <DsButton tone='primary' size='md' onClick={handleRunTask} disabled={running}>
              {running ? <CircularProgress size={16} sx={{ mr: 1 }} /> : null}
              {running ? 'Running...' : 'Run Task'}
            </DsButton>

            {/* Results Panel */}
            {error && (
              <Alert severity='error' sx={{ mt: 3 }}>
                {error}
              </Alert>
            )}
            {result !== null && (
              <Box sx={{ mt: 3 }}>
                <Typography variant='subtitle2' fontWeight={600} sx={{ mb: 1 }}>
                  Execution Output
                </Typography>
                <Box
                  component='pre'
                  sx={{
                    bgcolor: '#f5f5f5',
                    borderRadius: 1,
                    p: 2,
                    overflow: 'auto',
                    fontSize: 13,
                    maxHeight: 400,
                    border: '1px solid #e0e0e0',
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
