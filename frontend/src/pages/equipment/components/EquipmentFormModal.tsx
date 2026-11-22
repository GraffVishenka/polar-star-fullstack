/**
 * Создание и редактирование оборудования (EQ-2).
 *
 * Поля: наименование, тип, серийный и регистрационный номер, здание и склад.
 * Уникальность регистрационного номера проверяет сервер →
 * `409 DUPLICATE_REGISTRATION_NUMBER` подсвечивается прямо у поля.
 *
 * Смена склада при редактировании выполняется через `POST /equipment/{id}/move`:
 * так перемещение попадает в журнал (EQ-3/EQ-5) и не «теряется» в истории.
 */
import { useEffect, useState } from 'react';
import { Alert, Form, Input, Modal, Select } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { createEquipment, moveEquipment, updateEquipment } from '@/api/equipment';
import type { Building, Equipment, EquipmentType, Warehouse } from '@/api/types';
import { describeEquipmentError } from '../errors';
import { LocationFields } from './LocationFields';

interface FormValues {
  name: string;
  equipment_type_id: number;
  serial_number: string;
  registration_number: string;
  building_id: number;
  warehouse_id: number;
}

export interface EquipmentFormModalProps {
  open: boolean;
  /** `null` — создание новой единицы. */
  equipment: Equipment | null;
  buildings: Building[];
  warehouses: Warehouse[];
  types: EquipmentType[];
  isAdmin: boolean;
  onClose: () => void;
  onSaved: (equipment: Equipment) => void;
}

export function EquipmentFormModal({
  open,
  equipment,
  buildings,
  warehouses,
  types,
  isAdmin,
  onClose,
  onSaved,
}: EquipmentFormModalProps) {
  const [form] = Form.useForm<FormValues>();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setError(null);
    form.resetFields();
    form.setFieldsValue(
      equipment
        ? {
            name: equipment.name,
            equipment_type_id: equipment.equipment_type_id,
            serial_number: equipment.serial_number,
            registration_number: equipment.registration_number,
            building_id: equipment.building_id,
            warehouse_id: equipment.warehouse_id,
          }
        : { name: '', serial_number: '', registration_number: '' },
    );
  }, [open, equipment, form]);

  const mutation = useMutation({
    mutationFn: async (values: FormValues): Promise<Equipment> => {
      const payload = {
        name: values.name.trim(),
        equipment_type_id: values.equipment_type_id,
        serial_number: values.serial_number.trim(),
        registration_number: values.registration_number.trim(),
      };

      if (!equipment) {
        return createEquipment({ ...payload, warehouse_id: values.warehouse_id });
      }

      const updated = await updateEquipment(equipment.id, payload);
      if (values.warehouse_id === equipment.warehouse_id) return updated;

      // Местоположение меняем только через move: иначе журнал потеряет запись.
      return moveEquipment(equipment.id, {
        to_warehouse_id: values.warehouse_id,
        comment: 'Местоположение изменено в карточке оборудования',
      });
    },
    onSuccess: (saved) => {
      onSaved(saved);
      onClose();
    },
    onError: (cause) => {
      const message = describeEquipmentError(cause, 'save');
      setError(message);
      const code = (cause as { code?: string }).code;
      if (code === 'DUPLICATE_REGISTRATION_NUMBER') {
        form.setFields([{ name: 'registration_number', errors: [message] }]);
      }
    },
  });

  const submit = () => {
    setError(null);
    form
      .validateFields()
      .then((values) => mutation.mutate(values))
      .catch(() => undefined);
  };

  return (
    <Modal
      open={open}
      title={equipment ? 'Редактирование оборудования' : 'Новое оборудование'}
      width={560}
      okText={equipment ? 'Сохранить' : 'Создать'}
      cancelText="Отмена"
      confirmLoading={mutation.isPending}
      okButtonProps={{ 'data-testid': 'equipment-form-submit' }}
      cancelButtonProps={{ 'data-testid': 'equipment-form-cancel' }}
      onOk={submit}
      onCancel={onClose}
      maskClosable={false}
    >
      <div data-testid="equipment-form">
        {error ? (
          <Alert
            type="error"
            showIcon
            message={error}
            style={{ marginBottom: 12 }}
            data-testid="equipment-form-error"
          />
        ) : null}

        <Form form={form} layout="vertical" requiredMark={false} onFinish={submit}>
          <Form.Item
            label="Наименование"
            name="name"
            rules={[
              { required: true, message: 'Укажите наименование' },
              { max: 200, message: 'Не больше 200 символов' },
            ]}
          >
            <Input placeholder="Например: Ноутбук Dell Latitude" maxLength={200} showCount />
          </Form.Item>

          <Form.Item
            label="Тип оборудования"
            name="equipment_type_id"
            rules={[{ required: true, message: 'Выберите тип оборудования' }]}
          >
            <Select
              placeholder="Тип"
              options={types.map((type) => ({ value: type.id, label: type.name }))}
              data-testid="equipment-form-type"
            />
          </Form.Item>

          <Form.Item
            label="Серийный номер"
            name="serial_number"
            rules={[
              { required: true, message: 'Укажите серийный номер' },
              { max: 100, message: 'Не больше 100 символов' },
            ]}
          >
            <Input placeholder="SN-0001" maxLength={100} />
          </Form.Item>

          <Form.Item
            label="Регистрационный номер"
            name="registration_number"
            rules={[
              { required: true, message: 'Укажите регистрационный номер' },
              { max: 100, message: 'Не больше 100 символов' },
            ]}
            extra="Номер уникален среди неудалённого оборудования (EQ-2)"
          >
            <Input
              placeholder="INV-1001"
              maxLength={100}
              data-testid="equipment-form-registration"
            />
          </Form.Item>

          <LocationFields
            buildings={buildings}
            warehouses={warehouses}
            isAdmin={isAdmin}
            warehouseField="warehouse_id"
            warehouseExtra={
              equipment
                ? 'Смена помещения фиксируется в журнале перемещений (EQ-3)'
                : 'Оборудование сразу будет закреплено за помещением'
            }
          />
        </Form>
      </div>
    </Modal>
  );
}
