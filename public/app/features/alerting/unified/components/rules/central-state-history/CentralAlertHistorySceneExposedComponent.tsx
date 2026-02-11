import { Suspense, lazy } from 'react';

import { CentralAlertHistorySceneV1Props } from '@grafana/data';
import { t } from '@grafana/i18n';

const CentralAlertHistoryScene = lazy(() => import('./CentralAlertHistoryScene'));

const CentralAlertHistorySceneExposedComponent = (props: CentralAlertHistorySceneV1Props) => (
  <Suspense fallback={t('common.loading', 'Loading...')}>
    <CentralAlertHistoryScene {...props} />
  </Suspense>
);

export default CentralAlertHistorySceneExposedComponent;
