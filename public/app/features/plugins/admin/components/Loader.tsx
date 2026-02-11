import { t } from '@grafana/i18n';
import { Box, LoadingPlaceholder } from '@grafana/ui';

export interface Props {
  text?: string;
}

export const Loader = (opts: Props) => {
  const text = opts.text === undefined ? t('common.loading', 'Loading...') : opts.text;
  return (
    <Box display="flex" alignItems="center" direction="column" justifyContent="center" paddingTop={10}>
      <LoadingPlaceholder text={text} />
    </Box>
  );
};
