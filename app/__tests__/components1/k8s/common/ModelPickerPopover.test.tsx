import React from 'react';
import { render, screen, fireEvent, within } from '@testing-library/react';

// Mock @assets so importing TextAreaV2 doesn't pull image binaries through jest.
jest.mock('@assets', () => ({
  ArrowRightWhiteIcon: 'arrow-right-mock',
  CustomAgentBlueIcon: 'agent-blue-mock',
}));

jest.mock('@components1/llm/common/AgentIcon', () => ({
  getIcon: jest.fn(() => 'agent-icon-mock'),
}));

jest.mock('@components1/common/SafeIcon', () => ({
  __esModule: true,
  default: ({ alt }: { alt: string }) => <span data-testid='safe-icon'>{alt}</span>,
}));

jest.mock('@components1/ds/Toast', () => ({ toast: { success: jest.fn(), error: jest.fn() } }));

import { ModelPickerPopover } from '@components1/k8s/common/TextAreaV2';

const MODELS = [
  { provider: 'googleai', model: 'gemini-2.5-pro' },
  { provider: 'googleai', model: 'gemini-2.5-flash' },
  { provider: 'openai', model: 'gpt-4o-mini' },
];

describe('ModelPickerPopover — mutual exclusivity on Apply', () => {
  let onModelSelect: jest.Mock;
  let onTierModelsSelect: jest.Mock;

  beforeEach(() => {
    onModelSelect = jest.fn();
    onTierModelsSelect = jest.fn();
  });

  function openPicker() {
    fireEvent.click(screen.getByTestId('model-picker-trigger'));
  }

  it('All-calls mode: picking a model + Apply fires onModelSelect AND onTierModelsSelect(null)', () => {
    render(<ModelPickerPopover models={MODELS} onModelSelect={onModelSelect} onTierModelsSelect={onTierModelsSelect} />);
    openPicker();

    // Default mode is 'All calls' (blanket). Pick a model row.
    fireEvent.click(screen.getByText('gemini-2.5-pro'));
    fireEvent.click(screen.getByText('Apply'));

    expect(onTierModelsSelect).toHaveBeenCalledWith(null);
    expect(onModelSelect).toHaveBeenCalledWith({ provider: 'googleai', model: 'gemini-2.5-pro' });
  });

  it('By-task mode: picking per-tier + Apply fires onTierModelsSelect with picks AND onModelSelect(null)', () => {
    render(<ModelPickerPopover models={MODELS} onModelSelect={onModelSelect} onTierModelsSelect={onTierModelsSelect} />);
    openPicker();

    // Switch to By task mode.
    fireEvent.click(screen.getByText('By task'));

    // Default active tier is Reasoning — pick a model for it.
    fireEvent.click(screen.getByText('gemini-2.5-pro'));

    // Switch active tier to Retrieval. "Retrieval" also appears in the
    // summary row below, so scope by the tier-toggle group.
    const tierToggleGroup = screen.getByRole('group', { name: 'Active task' });
    fireEvent.click(within(tierToggleGroup).getByText('Retrieval'));
    fireEvent.click(screen.getByText('gpt-4o-mini'));

    fireEvent.click(screen.getByText('Apply'));

    expect(onModelSelect).toHaveBeenCalledWith(null);
    expect(onTierModelsSelect).toHaveBeenCalledTimes(1);
    expect(onTierModelsSelect).toHaveBeenCalledWith({
      reasoning: { provider: 'googleai', model: 'gemini-2.5-pro' },
      retrieval: { provider: 'openai', model: 'gpt-4o-mini' },
    });
  });

  it('Clear all fires both callbacks with null', () => {
    render(
      <ModelPickerPopover
        models={MODELS}
        selectedModel={{ provider: 'googleai', model: 'gemini-2.5-pro' }}
        onModelSelect={onModelSelect}
        onTierModelsSelect={onTierModelsSelect}
      />
    );
    openPicker();

    fireEvent.click(screen.getByText('Clear all'));

    expect(onModelSelect).toHaveBeenCalledWith(null);
    expect(onTierModelsSelect).toHaveBeenCalledWith(null);
  });

  it('Search input filters the model list', () => {
    render(<ModelPickerPopover models={MODELS} onModelSelect={onModelSelect} onTierModelsSelect={onTierModelsSelect} />);
    openPicker();

    const listbox = screen.getByRole('listbox');
    expect(within(listbox).queryByText('gpt-4o-mini')).toBeInTheDocument();
    expect(within(listbox).queryByText('gemini-2.5-pro')).toBeInTheDocument();

    const search = screen.getByPlaceholderText('Search models…') as HTMLInputElement;
    fireEvent.change(search, { target: { value: 'gpt' } });

    expect(within(listbox).queryByText('gpt-4o-mini')).toBeInTheDocument();
    expect(within(listbox).queryByText('gemini-2.5-pro')).not.toBeInTheDocument();
    expect(within(listbox).queryByText('gemini-2.5-flash')).not.toBeInTheDocument();
  });
});
